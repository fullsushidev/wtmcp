package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reUnsafeFileChars = regexp.MustCompile(`[<>:"/\\|?*]`)

// saveExportFile saves exported content to a local file.
// If outputPath is empty, saves to ./drive/<title>.md.
func saveExportFile(title, outputPath, content string) (string, error) {
	baseDir := "drive"

	if outputPath == "" {
		safeTitle := reUnsafeFileChars.ReplaceAllString(title, "_")
		safeTitle = filepath.Base(safeTitle)
		outputPath = filepath.Join(baseDir, safeTitle+".md")
	}

	// Validate that the resolved path stays within the base directory.
	// This must happen before any filesystem side effects.
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}
	if !strings.HasPrefix(absOutput, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("output path escapes base directory: %s", outputPath)
	}

	dir := filepath.Dir(absOutput)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	if err := os.WriteFile(absOutput, []byte(content), 0o600); err != nil {
		return "", err
	}

	return absOutput, nil
}
