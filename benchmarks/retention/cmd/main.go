package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"toron/benchmarks/retention"
)

func main() {
	action := flag.String("action", "", "Action to execute: init, archive, record")
	historyBase := flag.String("history-base", "benchmarks/results/history", "Base path for historical runs")
	canonicalDir := flag.String("canonical-dir", "benchmarks/results", "Base path for canonical latest results")
	timestamp := flag.String("timestamp", "", "Execution timestamp (YYYY-MM-DD_HH-MM-SS)")
	sessionName := flag.String("session-name", "", "Optional session name suffix")
	sessionDir := flag.String("session-dir", "", "Explicit target session directory")
	suite := flag.String("suite", "unknown", "Benchmark suite or subsystem name")
	status := flag.String("status", "success", "Completion status: success or failed")
	duration := flag.Float64("duration", 0.0, "Execution duration in seconds")
	command := flag.String("command", "", "Executed command string")
	artifacts := flag.String("artifacts", "", "Comma-separated list of artifact paths")
	paramsJSON := flag.String("params-json", "", "JSON string representing execution parameters")
	stagesJSON := flag.String("stages-json", "", "JSON string representing stages array")

	flag.Parse()

	cwd, _ := os.Getwd()

	switch *action {
	case "init":
		ts := *timestamp
		if ts == "" {
			ts = retention.FormatTimestamp(time.Now())
		}
		targetDir, dirName, err := retention.CreateHistoryDirectory(*historyBase, ts, *sessionName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating history directory: %v\n", err)
			os.Exit(1)
		}
		// Print the relative directory and directory name
		fmt.Printf("SESSION_DIR=%s\n", targetDir)
		fmt.Printf("SESSION_NAME=%s\n", dirName)
		fmt.Printf("SESSION_TS=%s\n", ts)

	case "archive":
		targetSessionDir := *sessionDir
		if targetSessionDir == "" {
			ts := *timestamp
			if ts == "" {
				ts = retention.FormatTimestamp(time.Now())
			}
			var err error
			targetSessionDir, _, err = retention.CreateHistoryDirectory(*historyBase, ts, *sessionName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error provisioning history directory: %v\n", err)
				os.Exit(1)
			}
		}

		var fileList []string
		if *artifacts != "" {
			for _, f := range strings.Split(*artifacts, ",") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					fileList = append(fileList, trimmed)
				}
			}
		}

		replicated, err := retention.ReplicateArtifacts(fileList, targetSessionDir, *canonicalDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning during artifact replication: %v\n", err)
		}

		// Parse parameters
		var params map[string]interface{}
		if *paramsJSON != "" {
			_ = json.Unmarshal([]byte(*paramsJSON), &params)
		}

		// Parse stages
		var stages []retention.StageRecord
		if *stagesJSON != "" {
			_ = json.Unmarshal([]byte(*stagesJSON), &stages)
		}

		commit, branch := retention.GetGitMetadata(cwd)
		goVersion := retention.GetGoVersion()
		tsNow := time.Now().Format(time.RFC3339)
		runID := fmt.Sprintf("run_%s", filepath.Base(targetSessionDir))

		// Relative directory from project root
		relDir, err := filepath.Rel(cwd, targetSessionDir)
		if err != nil || strings.HasPrefix(relDir, "..") {
			relDir = targetSessionDir
		}

		run := retention.RunRecord{
			RunID:           runID,
			Timestamp:       tsNow,
			Directory:       relDir,
			Suite:           *suite,
			Command:         *command,
			GitCommit:       commit,
			GitBranch:       branch,
			GoVersion:       goVersion,
			Status:          *status,
			DurationSeconds: *duration,
			Parameters:      params,
			Artifacts:       replicated,
			Stages:          stages,
		}

		meta := retention.SessionMeta{
			RunID:           runID,
			Timestamp:       tsNow,
			Suite:           *suite,
			Command:         *command,
			GitCommit:       commit,
			GitBranch:       branch,
			GoVersion:       goVersion,
			Status:          *status,
			DurationSeconds: *duration,
			Parameters:      params,
			Artifacts:       replicated,
			Stages:          stages,
		}

		if err := retention.WriteSessionMeta(targetSessionDir, meta); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing session meta: %v\n", err)
		}

		manifestPath := filepath.Join(*historyBase, "manifest.json")
		if err := retention.UpdateManifest(manifestPath, run); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating manifest: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Archived %d artifacts to %s\n", len(replicated), targetSessionDir)
		fmt.Printf("Updated manifest: %s\n", manifestPath)

	case "record":
		var stages []retention.StageRecord
		if *stagesJSON != "" {
			_ = json.Unmarshal([]byte(*stagesJSON), &stages)
		}
		var params map[string]interface{}
		if *paramsJSON != "" {
			_ = json.Unmarshal([]byte(*paramsJSON), &params)
		}
		var fileList []string
		if *artifacts != "" {
			for _, f := range strings.Split(*artifacts, ",") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					fileList = append(fileList, filepath.Base(trimmed))
				}
			}
		}

		commit, branch := retention.GetGitMetadata(cwd)
		goVersion := retention.GetGoVersion()
		tsNow := time.Now().Format(time.RFC3339)
		dirName := filepath.Base(*sessionDir)
		runID := fmt.Sprintf("run_%s", dirName)

		relDir, err := filepath.Rel(cwd, *sessionDir)
		if err != nil || strings.HasPrefix(relDir, "..") {
			relDir = *sessionDir
		}

		run := retention.RunRecord{
			RunID:           runID,
			Timestamp:       tsNow,
			Directory:       relDir,
			Suite:           *suite,
			Command:         *command,
			GitCommit:       commit,
			GitBranch:       branch,
			GoVersion:       goVersion,
			Status:          *status,
			DurationSeconds: *duration,
			Parameters:      params,
			Artifacts:       fileList,
			Stages:          stages,
		}

		manifestPath := filepath.Join(*historyBase, "manifest.json")
		if err := retention.UpdateManifest(manifestPath, run); err != nil {
			fmt.Fprintf(os.Stderr, "Error recording manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Recorded run %s in %s\n", runID, manifestPath)

	default:
		fmt.Fprintf(os.Stderr, "Unknown action '%s'. Supported: init, archive, record\n", *action)
		os.Exit(1)
	}
}
