package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// saveExportFile saves exported content to a local file.
// If outputPath is empty, saves to ./drive/<title>.md.
func saveExportFile(title, outputPath, content string) (string, error) {
	if outputPath == "" {
		driveDir := "drive"
		if err := os.MkdirAll(driveDir, 0o750); err != nil {
			return "", fmt.Errorf("create drive dir: %w", err)
		}
		safeTitle := regexp.MustCompile(`[<>:"/\\|?*]`).ReplaceAllString(title, "_")
		outputPath = filepath.Join(driveDir, safeTitle+".md")
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(content), 0o600); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return outputPath, err
	}
	return abs, nil
}
