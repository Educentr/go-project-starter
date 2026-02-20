// Package migrate provides config migration utilities
package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// DeprecationWarning represents a deprecation warning
type DeprecationWarning struct {
	Feature       string
	Description   string
	RemovalVer    string
	MigrationHint string
}

// MigrationResult contains the result of a migration
type MigrationResult struct {
	Modified     bool
	Warnings     []DeprecationWarning
	OriginalPath string
	BackupPath   string
}

// Migrator handles config migrations
type Migrator struct {
	configPath string
	dryRun     bool
}

// Version constants for deprecation tracking
const (
	// CurrentVersion is the current generator version
	CurrentVersion = "0.10.0"
)

// Deprecation removal version constants - each deprecation has its own removal version
// (currently none pending)

// Error message constants
const (
	errMsgReadConfigFile  = "failed to read config file"
	errMsgParseConfigFile = "failed to parse config file"
)

// File permission for config files
const configFilePermission = 0o600

// New creates a new Migrator
func New(configPath string, dryRun bool) *Migrator {
	return &Migrator{
		configPath: configPath,
		dryRun:     dryRun,
	}
}

// Migrate performs the migration and returns the result
func (m *Migrator) Migrate() (*MigrationResult, error) {
	result := &MigrationResult{
		OriginalPath: m.configPath,
	}

	// No migrations currently needed — transport string array format removed in v0.12.0
	return result, nil
}

// CheckDeprecations checks config for deprecated features without migrating
func CheckDeprecations(configPath string) ([]DeprecationWarning, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrap(err, errMsgReadConfigFile)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, errMsgParseConfigFile)
	}

	var warnings []DeprecationWarning

	return warnings, nil
}

// PrintWarnings prints deprecation warnings to stderr
func PrintWarnings(warnings []DeprecationWarning) {
	if len(warnings) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr, "\n⚠️  DEPRECATION WARNINGS:")
	fmt.Fprintln(os.Stderr, "========================")

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "\n[DEPRECATED] %s\n", w.Feature)
		fmt.Fprintf(os.Stderr, "  Description: %s\n", w.Description)
		fmt.Fprintf(os.Stderr, "  Will be removed in: %s\n", w.RemovalVer)
		fmt.Fprintf(os.Stderr, "  Migration: %s\n", w.MigrationHint)
	}

	fmt.Fprintln(os.Stderr)
}

// FindConfigFile finds the config file path
func FindConfigFile(configDir, configFile string) string {
	// Try configDir/configFile first
	path := filepath.Join(configDir, configFile)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Try just configFile
	if _, err := os.Stat(configFile); err == nil {
		return configFile
	}

	// Try default location
	defaultPath := filepath.Join(".project-config", "project.yaml")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}

	return path // Return the first attempt path for error message
}
