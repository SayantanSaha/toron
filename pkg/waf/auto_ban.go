package waf

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BanType represents the tier of ban applied to an IP address.
type BanType string

const (
	BanTypeTemporary BanType = "temporary"
	BanTypePermanent BanType = "permanent"
)

// BanEntry records an active temporary or permanent ban for a client IP.
type BanEntry struct {
	IP           string    `json:"ip"`
	Type         BanType   `json:"type"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	TempBanCount int       `json:"temp_ban_count"`
	Reason       string    `json:"reason"`
	LastCategory string    `json:"last_category,omitempty"`
}

// RemainingDuration returns remaining TTL for temporary bans, or 0 for permanent bans.
func (b *BanEntry) RemainingDuration() time.Duration {
	if b == nil || b.Type == BanTypePermanent || b.ExpiresAt.IsZero() {
		return 0
	}
	rem := time.Until(b.ExpiresAt)
	if rem < 0 {
		return 0
	}
	return rem
}

// StrikeRecord tracks sliding-window security violations and ban escalation counts for an IP.
type StrikeRecord struct {
	IP           string      `json:"ip"`
	Timestamps   []time.Time `json:"timestamps"`
	TempBanCount int         `json:"temp_ban_count"`
	LastCategory string      `json:"last_category,omitempty"`
}

// AutoBanConfig holds configuration options for the 2-Stage Auto-Ban Engine.
type AutoBanConfig struct {
	Enabled          bool          `json:"enabled" yaml:"enabled"`
	MaxViolations    int           `json:"max_violations" yaml:"max_violations"`
	Window           time.Duration `json:"window" yaml:"window"`
	BanDuration      time.Duration `json:"ban_duration" yaml:"ban_duration"`
	MaxTemporaryBans int           `json:"max_temporary_bans" yaml:"max_temporary_bans"`
	PersistenceFile  string        `json:"persistence_file" yaml:"persistence_file"`
	Whitelist        []string      `json:"whitelist" yaml:"whitelist"`
}

// DefaultAutoBanConfig returns production defaults for the Auto-Ban engine.
func DefaultAutoBanConfig() AutoBanConfig {
	return AutoBanConfig{
		Enabled:          false,
		MaxViolations:    5,
		Window:           1 * time.Minute,
		BanDuration:      1 * time.Hour,
		MaxTemporaryBans: 3,
		PersistenceFile:  "/etc/toron/banned_ips.json",
		Whitelist: []string{
			"127.0.0.1/32",
			"::1/128",
		},
	}
}

// PersistentBanState represents the JSON schema written to disk.
type PersistentBanState struct {
	Version   int                      `json:"version"`
	UpdatedAt time.Time                `json:"updated_at"`
	Bans      map[string]*BanEntry     `json:"bans"`
	Strikes   map[string]*StrikeRecord `json:"strikes,omitempty"`
}

// AutoBanManager coordinates in-memory strike tracking, 2-stage ban escalation, and disk persistence.
type AutoBanManager struct {
	cfg             AutoBanConfig
	mu              sync.RWMutex
	bans            map[string]*BanEntry
	strikes         map[string]*StrikeRecord
	whitelistNets   []*net.IPNet
	whitelistIPs    []net.IP
	auditLogger     *AuditLogger
	stopChan        chan struct{}
	persistencePath string
}

// NewAutoBanManager initializes the 2-stage auto-ban engine with persistence and background sweeper.
func NewAutoBanManager(cfg AutoBanConfig, logger *AuditLogger) (*AutoBanManager, error) {
	if cfg.MaxViolations <= 0 {
		cfg.MaxViolations = 5
	}
	if cfg.Window <= 0 {
		cfg.Window = 1 * time.Minute
	}
	if cfg.BanDuration <= 0 {
		cfg.BanDuration = 1 * time.Hour
	}
	if cfg.MaxTemporaryBans <= 0 {
		cfg.MaxTemporaryBans = 3
	}

	mgr := &AutoBanManager{
		cfg:             cfg,
		bans:            make(map[string]*BanEntry),
		strikes:         make(map[string]*StrikeRecord),
		auditLogger:     logger,
		stopChan:        make(chan struct{}),
		persistencePath: strings.TrimSpace(cfg.PersistenceFile),
	}

	mgr.compileWhitelist(cfg.Whitelist)

	if mgr.persistencePath != "" {
		_ = mgr.loadState()
	}

	// Start background cleanup sweeper for expired temporary bans
	go mgr.sweepLoop()

	return mgr, nil
}

func (m *AutoBanManager) compileWhitelist(whitelist []string) {
	var nets []*net.IPNet
	var ips []net.IP

	// Always guarantee loopback addresses are immune
	loopback4 := net.ParseIP("127.0.0.1")
	loopback6 := net.ParseIP("::1")
	if loopback4 != nil {
		ips = append(ips, loopback4)
	}
	if loopback6 != nil {
		ips = append(ips, loopback6)
	}

	for _, raw := range whitelist {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			_, ipNet, err := net.ParseCIDR(raw)
			if err == nil && ipNet != nil {
				nets = append(nets, ipNet)
			}
		} else {
			ip := net.ParseIP(raw)
			if ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	m.whitelistNets = nets
	m.whitelistIPs = ips
}

// IsWhitelisted checks if an IP is protected from strikes and auto-bans.
func (m *AutoBanManager) IsWhitelisted(ipStr string) bool {
	if ipStr == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}

	for _, nip := range m.whitelistIPs {
		if nip.Equal(ip) {
			return true
		}
	}
	for _, ipNet := range m.whitelistNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// IsEnabled returns whether auto-banning is active.
func (m *AutoBanManager) IsEnabled() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Enabled
}

// IsBanned checks whether an IP is actively banned (Stage 1 or Stage 2).
func (m *AutoBanManager) IsBanned(ipStr string) (bool, *BanEntry) {
	if m == nil || ipStr == "" {
		return false, nil
	}

	m.mu.RLock()
	if !m.cfg.Enabled {
		m.mu.RUnlock()
		return false, nil
	}
	m.mu.RUnlock()

	if m.IsWhitelisted(ipStr) {
		return false, nil
	}

	m.mu.RLock()
	entry, exists := m.bans[ipStr]
	if !exists {
		m.mu.RUnlock()
		return false, nil
	}

	if entry.Type == BanTypePermanent {
		cp := *entry
		m.mu.RUnlock()
		return true, &cp
	}

	now := time.Now()
	if now.Before(entry.ExpiresAt) {
		cp := *entry
		m.mu.RUnlock()
		return true, &cp
	}
	m.mu.RUnlock()

	// Expired temporary ban: clean up under write lock
	m.mu.Lock()
	if cur, ok := m.bans[ipStr]; ok && cur.Type == BanTypeTemporary && !now.Before(cur.ExpiresAt) {
		delete(m.bans, ipStr)
		_ = m.saveStateLocked()
	}
	m.mu.Unlock()

	return false, nil
}

// RecordViolation records a strike for an IP and performs 2-stage ban escalation if thresholds are met.
func (m *AutoBanManager) RecordViolation(ipStr string, category string) (banned bool, entry *BanEntry) {
	if m == nil || ipStr == "" {
		return false, nil
	}

	m.mu.RLock()
	if !m.cfg.Enabled {
		m.mu.RUnlock()
		return false, nil
	}
	m.mu.RUnlock()

	if m.IsWhitelisted(ipStr) {
		return false, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// If already actively banned, return current ban
	if existing, exists := m.bans[ipStr]; exists {
		if existing.Type == BanTypePermanent || now.Before(existing.ExpiresAt) {
			cp := *existing
			return true, &cp
		}
		// Expired ban cleanup
		delete(m.bans, ipStr)
	}

	strike, exists := m.strikes[ipStr]
	if !exists {
		strike = &StrikeRecord{
			IP: ipStr,
		}
		m.strikes[ipStr] = strike
	}

	// Filter timestamps within sliding window
	windowStart := now.Add(-m.cfg.Window)
	validTimestamps := make([]time.Time, 0, len(strike.Timestamps)+1)
	for _, ts := range strike.Timestamps {
		if ts.After(windowStart) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	validTimestamps = append(validTimestamps, now)
	strike.Timestamps = validTimestamps
	strike.LastCategory = category

	if len(validTimestamps) >= m.cfg.MaxViolations {
		strike.TempBanCount++
		strike.Timestamps = nil // Clear strikes for current ban cycle

		var newBan *BanEntry
		if strike.TempBanCount >= m.cfg.MaxTemporaryBans {
			// Stage 2: Permanent Ban Escalation
			newBan = &BanEntry{
				IP:           ipStr,
				Type:         BanTypePermanent,
				CreatedAt:    now,
				TempBanCount: strike.TempBanCount,
				Reason:       fmt.Sprintf("Repeated security violations exceeded %d temporary bans (escalated to permanent ban)", m.cfg.MaxTemporaryBans),
				LastCategory: category,
			}
		} else {
			// Stage 1: Temporary Ban
			newBan = &BanEntry{
				IP:           ipStr,
				Type:         BanTypeTemporary,
				CreatedAt:    now,
				ExpiresAt:    now.Add(m.cfg.BanDuration),
				TempBanCount: strike.TempBanCount,
				Reason:       fmt.Sprintf("Accumulated %d security violations within %s", m.cfg.MaxViolations, m.cfg.Window),
				LastCategory: category,
			}
		}

		m.bans[ipStr] = newBan
		_ = m.saveStateLocked()

		if m.auditLogger != nil {
			m.auditLogger.LogEvent(SecurityEvent{
				Event:        "waf_auto_ban",
				ClientIP:     ipStr,
				Category:     category,
				AnomalyScore: 0,
				Action:       "banned",
				PayloadSnippet: fmt.Sprintf("Auto-ban applied: tier=%s, temp_ban_count=%d, reason=%s",
					newBan.Type, newBan.TempBanCount, newBan.Reason),
			})
		}

		cp := *newBan
		return true, &cp
	}

	return false, nil
}

// Ban manually adds an IP to the ban list (temporary or permanent).
func (m *AutoBanManager) Ban(ipStr string, banType BanType, duration time.Duration, reason string) error {
	if m == nil {
		return fmt.Errorf("auto ban manager is nil")
	}
	ipStr = strings.TrimSpace(ipStr)
	if net.ParseIP(ipStr) == nil {
		return fmt.Errorf("invalid IP address %q", ipStr)
	}

	if m.IsWhitelisted(ipStr) {
		return fmt.Errorf("cannot ban whitelisted IP %q", ipStr)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	entry := &BanEntry{
		IP:        ipStr,
		Type:      banType,
		CreatedAt: now,
		Reason:    reason,
	}

	if banType == BanTypeTemporary {
		if duration <= 0 {
			duration = m.cfg.BanDuration
		}
		entry.ExpiresAt = now.Add(duration)
	}

	m.bans[ipStr] = entry
	return m.saveStateLocked()
}

// Unban removes an IP from the ban table and resets its strike counter.
func (m *AutoBanManager) Unban(ipStr string) error {
	if m == nil {
		return fmt.Errorf("auto ban manager is nil")
	}
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return fmt.Errorf("empty IP address")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.bans, ipStr)
	delete(m.strikes, ipStr)

	return m.saveStateLocked()
}

// ListBanned returns a snapshot of all active temporary and permanent bans.
func (m *AutoBanManager) ListBanned() []BanEntry {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	res := make([]BanEntry, 0, len(m.bans))
	for _, b := range m.bans {
		if b.Type == BanTypePermanent || now.Before(b.ExpiresAt) {
			res = append(res, *b)
		}
	}
	return res
}

// Close stops the background cleanup worker and syncs state to disk.
func (m *AutoBanManager) Close() error {
	if m == nil {
		return nil
	}

	select {
	case <-m.stopChan:
		// already closed
	default:
		close(m.stopChan)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveStateLocked()
}

func (m *AutoBanManager) sweepLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.sweepExpiredBans()
		}
	}
}

func (m *AutoBanManager) sweepExpiredBans() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	changed := false

	for ip, b := range m.bans {
		if b.Type == BanTypeTemporary && !b.ExpiresAt.IsZero() && now.After(b.ExpiresAt) {
			delete(m.bans, ip)
			changed = true
		}
	}

	if changed {
		_ = m.saveStateLocked()
	}
}

func (m *AutoBanManager) saveStateLocked() error {
	if m.persistencePath == "" {
		return nil
	}

	state := PersistentBanState{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Bans:      m.bans,
		Strikes:   m.strikes,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal auto-ban state: %w", err)
	}

	dir := filepath.Dir(m.persistencePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create persistence directory %q: %w", dir, err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", m.persistencePath, os.Getpid())
	f, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp persistence file %q: %w", tmpFile, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to write persistence data: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to sync persistence file: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to close persistence file: %w", err)
	}

	if err := os.Rename(tmpFile, m.persistencePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to atomically rename persistence file: %w", err)
	}

	return nil
}

func (m *AutoBanManager) loadState() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.persistencePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(m.persistencePath)
	if err != nil {
		return fmt.Errorf("failed to read auto-ban state file %q: %w", m.persistencePath, err)
	}

	var state PersistentBanState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal auto-ban state file %q: %w", m.persistencePath, err)
	}

	now := time.Now()
	if state.Bans != nil {
		for ip, b := range state.Bans {
			if b.Type == BanTypePermanent || now.Before(b.ExpiresAt) {
				m.bans[ip] = b
			}
		}
	}

	if state.Strikes != nil {
		m.strikes = state.Strikes
	}

	return nil
}
