package waf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAutoBan_SlidingWindowAndStage1Ban(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "banned_ips.json")

	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    3,
		Window:           500 * time.Millisecond,
		BanDuration:      1 * time.Second,
		MaxTemporaryBans: 3,
		PersistenceFile:  persistPath,
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error creating auto-ban manager: %v", err)
	}
	defer mgr.Close()

	ip := "198.51.100.10"

	// 1st violation
	banned, _ := mgr.RecordViolation(ip, "sqli")
	if banned {
		t.Fatalf("expected not banned on 1st violation")
	}

	// 2nd violation
	banned, _ = mgr.RecordViolation(ip, "sqli")
	if banned {
		t.Fatalf("expected not banned on 2nd violation")
	}

	// 3rd violation -> Stage 1 Temporary Ban
	banned, entry := mgr.RecordViolation(ip, "sqli")
	if !banned || entry == nil {
		t.Fatalf("expected banned on 3rd violation")
	}
	if entry.Type != BanTypeTemporary {
		t.Fatalf("expected temporary ban, got %s", entry.Type)
	}
	if entry.TempBanCount != 1 {
		t.Fatalf("expected temp ban count 1, got %d", entry.TempBanCount)
	}

	// Fast path check
	isBanned, e2 := mgr.IsBanned(ip)
	if !isBanned || e2 == nil {
		t.Fatalf("expected IsBanned to return true")
	}

	// Wait for ban duration to expire
	time.Sleep(1100 * time.Millisecond)

	isBanned, _ = mgr.IsBanned(ip)
	if isBanned {
		t.Fatalf("expected ban to have expired")
	}
}

func TestAutoBan_Stage2PermanentEscalation(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "banned_ips.json")

	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    2,
		Window:           1 * time.Second,
		BanDuration:      100 * time.Millisecond,
		MaxTemporaryBans: 2, // 2nd temporary ban will escalate to permanent
		PersistenceFile:  persistPath,
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Close()

	ip := "203.0.113.50"

	// Cycle 1: 2 strikes -> Temp Ban 1
	mgr.RecordViolation(ip, "traversal")
	banned, entry := mgr.RecordViolation(ip, "traversal")
	if !banned || entry.Type != BanTypeTemporary || entry.TempBanCount != 1 {
		t.Fatalf("expected temp ban 1, got: %+v", entry)
	}

	// Wait for temp ban 1 to expire
	time.Sleep(150 * time.Millisecond)
	if isBanned, _ := mgr.IsBanned(ip); isBanned {
		t.Fatalf("expected temp ban 1 to expire")
	}

	// Cycle 2: 2 strikes -> Escalates to Permanent Ban
	mgr.RecordViolation(ip, "traversal")
	banned, entry = mgr.RecordViolation(ip, "traversal")
	if !banned || entry.Type != BanTypePermanent || entry.TempBanCount != 2 {
		t.Fatalf("expected permanent ban escalation, got: %+v", entry)
	}

	// Verify permanence
	time.Sleep(200 * time.Millisecond)
	isBanned, e := mgr.IsBanned(ip)
	if !isBanned || e.Type != BanTypePermanent {
		t.Fatalf("expected permanent ban to persist without expiring")
	}
}

func TestAutoBan_WhitelistImmunity(t *testing.T) {
	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    1,
		Window:           1 * time.Minute,
		BanDuration:      1 * time.Hour,
		MaxTemporaryBans: 1,
		Whitelist: []string{
			"127.0.0.1/32",
			"10.0.0.0/8",
			"192.168.1.100",
		},
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Close()

	testIPs := []string{"127.0.0.1", "10.1.2.3", "192.168.1.100", "::1"}

	for _, ip := range testIPs {
		for i := 0; i < 5; i++ {
			banned, _ := mgr.RecordViolation(ip, "sqli")
			if banned {
				t.Fatalf("whitelisted IP %s was banned", ip)
			}
		}
		if isBanned, _ := mgr.IsBanned(ip); isBanned {
			t.Fatalf("whitelisted IP %s returned true for IsBanned", ip)
		}
	}
}

func TestAutoBan_AtomicPersistenceAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "sub", "banned_ips.json")

	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    2,
		Window:           1 * time.Minute,
		BanDuration:      1 * time.Hour,
		MaxTemporaryBans: 3,
		PersistenceFile:  persistPath,
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// 1 Temp Ban and 1 Permanent Ban
	_ = mgr.Ban("198.51.100.22", BanTypeTemporary, 2*time.Hour, "Manual temp ban")
	_ = mgr.Ban("198.51.100.33", BanTypePermanent, 0, "Manual perm ban")

	// Close manager (syncs state to disk)
	_ = mgr.Close()

	// Verify file exists on disk
	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Fatalf("expected persistence file to exist at %s", persistPath)
	}

	// Instantiate new manager pointing to same file
	mgr2, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create second manager: %v", err)
	}
	defer mgr2.Close()

	// Verify bans restored
	b1, e1 := mgr2.IsBanned("198.51.100.22")
	if !b1 || e1 == nil || e1.Type != BanTypeTemporary {
		t.Fatalf("expected restored temp ban for 198.51.100.22")
	}

	b2, e2 := mgr2.IsBanned("198.51.100.33")
	if !b2 || e2 == nil || e2.Type != BanTypePermanent {
		t.Fatalf("expected restored perm ban for 198.51.100.33")
	}

	// Unban IP
	if err := mgr2.Unban("198.51.100.22"); err != nil {
		t.Fatalf("failed to unban: %v", err)
	}
	b1After, _ := mgr2.IsBanned("198.51.100.22")
	if b1After {
		t.Fatalf("expected 198.51.100.22 to be unbanned")
	}
}

func TestAutoBan_ConcurrencyRaceFree(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "banned_ips.json")

	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    5,
		Window:           1 * time.Second,
		BanDuration:      500 * time.Millisecond,
		MaxTemporaryBans: 2,
		PersistenceFile:  persistPath,
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Close()

	var wg sync.WaitGroup
	workers := 20
	iterations := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ip := fmt.Sprintf("198.51.100.%d", workerID%5)
			for i := 0; i < iterations; i++ {
				mgr.RecordViolation(ip, "bot")
				mgr.IsBanned(ip)
				if i%20 == 0 {
					_ = mgr.ListBanned()
				}
			}
		}(w)
	}

	wg.Wait()
}

func TestAutoBanManager_UpdateConfig(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "banned_ips.json")

	cfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    3,
		Window:           1 * time.Minute,
		BanDuration:      1 * time.Hour,
		MaxTemporaryBans: 2,
		PersistenceFile:  persistPath,
		Whitelist:        []string{"10.0.0.1"},
	}

	mgr, err := NewAutoBanManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Ban an IP
	err = mgr.Ban("198.51.100.50", BanTypeTemporary, 1*time.Hour, "testing initial ban")
	if err != nil {
		t.Fatalf("failed to ban: %v", err)
	}

	// Update config in place
	newCfg := AutoBanConfig{
		Enabled:          true,
		MaxViolations:    1,
		Window:           30 * time.Second,
		BanDuration:      2 * time.Hour,
		MaxTemporaryBans: 1,
		PersistenceFile:  persistPath,
		Whitelist:        []string{"10.0.0.1", "10.0.0.2"},
	}
	mgr.UpdateConfig(newCfg, nil)

	// Verify pre-existing ban is preserved
	isBanned, entry := mgr.IsBanned("198.51.100.50")
	if !isBanned || entry == nil || entry.Reason != "testing initial ban" {
		t.Fatalf("expected pre-existing ban to be preserved across UpdateConfig, got %v", isBanned)
	}

	// Verify updated whitelist is active
	if !mgr.IsWhitelisted("10.0.0.2") {
		t.Fatalf("expected 10.0.0.2 to be whitelisted after UpdateConfig")
	}

	// Verify unban works on the same manager
	if err := mgr.Unban("198.51.100.50"); err != nil {
		t.Fatalf("failed to unban: %v", err)
	}
	isBanned, _ = mgr.IsBanned("198.51.100.50")
	if isBanned {
		t.Fatalf("expected 198.51.100.50 to be unbanned")
	}
}

