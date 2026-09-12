package retention

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ManifestSchemaVersion is the current version of the manifest format.
const ManifestSchemaVersion = "1.0"

// TimestampFormat is the deterministic format for historical directories.
const TimestampFormat = "2006-01-02_15-04-05"

var fileLock sync.Mutex

// StageRecord captures telemetry and outputs for an individual benchmark stage.
type StageRecord struct {
	Name            string                 `json:"name"`
	Status          string                 `json:"status"` // "success" | "failed"
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	Artifacts       []string               `json:"artifacts"`
	DurationSeconds float64                `json:"duration_seconds,omitempty"`
}

// RunRecord represents a single benchmark execution session recorded in manifest.json.
type RunRecord struct {
	RunID           string                 `json:"run_id"`
	Timestamp       string                 `json:"timestamp"`
	Directory       string                 `json:"directory"`
	Suite           string                 `json:"suite"`
	Command         string                 `json:"command,omitempty"`
	GitCommit       string                 `json:"git_commit,omitempty"`
	GitBranch       string                 `json:"git_branch,omitempty"`
	GoVersion       string                 `json:"go_version,omitempty"`
	Status          string                 `json:"status"` // "success" | "failed"
	DurationSeconds float64                `json:"duration_seconds"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	Artifacts       []string               `json:"artifacts,omitempty"`
	Stages          []StageRecord          `json:"stages,omitempty"`
}

// Manifest represents the index of all historical benchmark runs.
type Manifest struct {
	SchemaVersion string      `json:"schema_version"`
	UpdatedAt     string      `json:"updated_at"`
	TotalRuns     int         `json:"total_runs"`
	Runs          []RunRecord `json:"runs"`
}

// SessionMeta represents the self-contained metadata file inside each run directory.
type SessionMeta struct {
	RunID           string                 `json:"run_id"`
	Timestamp       string                 `json:"timestamp"`
	Suite           string                 `json:"suite"`
	Command         string                 `json:"command,omitempty"`
	GitCommit       string                 `json:"git_commit,omitempty"`
	GitBranch       string                 `json:"git_branch,omitempty"`
	GoVersion       string                 `json:"go_version,omitempty"`
	Status          string                 `json:"status"`
	DurationSeconds float64                `json:"duration_seconds"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	Artifacts       []string               `json:"artifacts,omitempty"`
	Stages          []StageRecord          `json:"stages,omitempty"`
}

// FormatTimestamp returns a formatted timestamp string from time.Time.
func FormatTimestamp(t time.Time) string {
	return t.Format(TimestampFormat)
}

// CreateHistoryDirectory creates a unique directory under historyBaseDir.
// If sessionName is provided, it is appended to the timestamp.
// If a collision occurs, an incremented suffix (_1, _2) is appended.
func CreateHistoryDirectory(historyBaseDir string, timestamp string, sessionName string) (string, string, error) {
	if err := os.MkdirAll(historyBaseDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create base history directory: %w", err)
	}

	dirName := timestamp
	if sessionName != "" {
		dirName = fmt.Sprintf("%s_%s", timestamp, sessionName)
	}

	targetPath := filepath.Join(historyBaseDir, dirName)
	if _, err := os.Stat(targetPath); err == nil {
		counter := 1
		for {
			collisionDir := fmt.Sprintf("%s_%d", dirName, counter)
			candidate := filepath.Join(historyBaseDir, collisionDir)
			if _, statErr := os.Stat(candidate); os.IsNotExist(statErr) {
				targetPath = candidate
				dirName = collisionDir
				break
			}
			counter++
		}
	}

	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create historical run directory %s: %w", targetPath, err)
	}

	return targetPath, dirName, nil
}

// CopyFile copies a single file from src to dst.
func CopyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source file %s stat error: %w", src, err)
	}

	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory for %s: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to open destination %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", src, dst, err)
	}

	return out.Sync()
}

// ReplicateArtifacts copies source files into historyDir and ensures canonicalDir has the latest copies.
// Returns the list of relative artifact basenames successfully replicated.
func ReplicateArtifacts(filePaths []string, historyDir, canonicalDir string) ([]string, error) {
	var copied []string

	for _, p := range filePaths {
		if strings.TrimSpace(p) == "" {
			continue
		}

		cleanPath := filepath.Clean(p)
		if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
			continue
		}

		baseName := filepath.Base(cleanPath)

		// Copy to historyDir
		histDest := filepath.Join(historyDir, baseName)
		if cleanPath != histDest {
			if err := CopyFile(cleanPath, histDest); err != nil {
				return copied, fmt.Errorf("failed to copy artifact %s to history: %w", baseName, err)
			}
		}

		// Ensure canonicalDir has the copy
		canonDest := filepath.Join(canonicalDir, baseName)
		if cleanPath != canonDest {
			if err := CopyFile(cleanPath, canonDest); err != nil {
				return copied, fmt.Errorf("failed to copy artifact %s to canonical dir: %w", baseName, err)
			}
		}

		copied = append(copied, baseName)
	}

	return copied, nil
}

// UpdateManifest updates manifest.json atomically by appending the given RunRecord.
func UpdateManifest(manifestPath string, run RunRecord) error {
	fileLock.Lock()
	defer fileLock.Unlock()

	dir := filepath.Dir(manifestPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Runs:          []RunRecord{},
	}

	if data, err := os.ReadFile(manifestPath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &manifest)
	}

	manifest.SchemaVersion = ManifestSchemaVersion
	manifest.UpdatedAt = time.Now().Format(time.RFC3339)
	manifest.Runs = append(manifest.Runs, run)
	manifest.TotalRuns = len(manifest.Runs)

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode manifest JSON: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d_%d", manifestPath, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, encoded, 0644); err != nil {
		return fmt.Errorf("failed to write temp manifest file: %w", err)
	}

	if err := os.Rename(tmpPath, manifestPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically rename temp manifest to %s: %w", manifestPath, err)
	}

	return nil
}

// WriteSessionMeta writes session_meta.json in historyDir.
func WriteSessionMeta(historyDir string, meta SessionMeta) error {
	metaPath := filepath.Join(historyDir, "session_meta.json")

	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode session_meta JSON: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d_%d", metaPath, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, encoded, 0644); err != nil {
		return fmt.Errorf("failed to write temp session_meta: %w", err)
	}

	if err := os.Rename(tmpPath, metaPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically rename session_meta: %w", err)
	}

	return nil
}

// GetGitMetadata attempts to retrieve the current git commit hash and branch.
func GetGitMetadata(workDir string) (commit, branch string) {
	cmdCommit := exec.Command("git", "rev-parse", "HEAD")
	cmdCommit.Dir = workDir
	if out, err := cmdCommit.Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	} else {
		commit = "unknown"
	}

	cmdBranch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmdBranch.Dir = workDir
	if out, err := cmdBranch.Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	} else {
		branch = "unknown"
	}

	return commit, branch
}

// GetGoVersion returns the Go runtime version.
func GetGoVersion() string {
	return runtime.Version()
}
