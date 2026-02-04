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
const (
	// RemovalVersionEmptyConfigAvailable is when empty_config_available will be removed
	RemovalVersionEmptyConfigAvailable = "0.13.0"
)

// Error message constants
const (
	errMsgReadConfigFile  = "failed to read config file"
	errMsgParseConfigFile = "failed to parse config file"
)

// Deprecation description templates
const (
	deprecationDescEmptyConfigAvailable = "%s '%s' uses deprecated 'empty_config_available'. Use 'optional: true' in application transport config instead"
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

	// Check empty_config_available in REST section
	if restList, ok := config["rest"].([]any); ok {
		for _, restRaw := range restList {
			if rest, ok := restRaw.(map[string]any); ok {
				if eca, ok := rest["empty_config_available"].(bool); ok && eca {
					name, _ := rest["name"].(string)
					warnings = append(warnings, DeprecationWarning{
						Feature:       "empty_config_available",
						Description:   fmt.Sprintf(deprecationDescEmptyConfigAvailable, "REST", name),
						RemovalVer:    RemovalVersionEmptyConfigAvailable,
						MigrationHint: "Remove 'empty_config_available' from REST config and add 'config: { optional: true }' to the transport in applications section",
					})
				}
			}
		}
	}

	// Check empty_config_available in gRPC section
	if grpcList, ok := config["grpc"].([]any); ok {
		for _, grpcRaw := range grpcList {
			if grpc, ok := grpcRaw.(map[string]any); ok {
				if eca, ok := grpc["empty_config_available"].(bool); ok && eca {
					name, _ := grpc["name"].(string)
					warnings = append(warnings, DeprecationWarning{
						Feature:       "empty_config_available",
						Description:   fmt.Sprintf(deprecationDescEmptyConfigAvailable, "gRPC", name),
						RemovalVer:    RemovalVersionEmptyConfigAvailable,
						MigrationHint: "Remove 'empty_config_available' from gRPC config and add 'config: { optional: true }' to the transport in applications section",
					})
				}
			}
		}
	}

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
