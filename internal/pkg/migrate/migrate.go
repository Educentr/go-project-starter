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
	// RemovalVersionTransportStringArray is when transport string array format will be removed
	RemovalVersionTransportStringArray = "0.12.0"
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
	deprecationDescTransportFormat        = "Application '%s' uses deprecated string array format for transports"
	deprecationDescEmptyConfigAvailable   = "%s '%s' uses deprecated 'empty_config_available'. Use 'optional: true' in application transport config instead"
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

	// Read the config file
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, errors.Wrap(err, errMsgReadConfigFile)
	}

	// Parse as generic YAML to preserve structure
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, errMsgParseConfigFile)
	}

	// Migrate transport format in applications
	modified, warnings := m.migrateApplicationTransports(config)
	result.Modified = modified
	result.Warnings = warnings

	if !modified {
		return result, nil
	}

	if m.dryRun {
		fmt.Println("Dry run mode - no changes will be written")
		m.printMigrationPlan(config)

		return result, nil
	}

	// Create backup
	backupPath := m.configPath + ".bak"
	if err := os.WriteFile(backupPath, data, configFilePermission); err != nil {
		return nil, errors.Wrap(err, "failed to create backup")
	}

	result.BackupPath = backupPath

	// Write migrated config
	newData, err := yaml.Marshal(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal migrated config")
	}

	if err := os.WriteFile(m.configPath, newData, configFilePermission); err != nil {
		return nil, errors.Wrap(err, "failed to write migrated config")
	}

	return result, nil
}

// migrateApplicationTransports migrates old transport format to new format
func (m *Migrator) migrateApplicationTransports(config map[string]any) (bool, []DeprecationWarning) {
	var warnings []DeprecationWarning

	modified := false

	apps, ok := config["applications"].([]any)
	if !ok {
		return false, nil
	}

	for i, appRaw := range apps {
		app, ok := appRaw.(map[string]any)
		if !ok {
			continue
		}

		transports, ok := app["transport"].([]any)
		if !ok {
			continue
		}

		newTransports := make([]any, 0, len(transports))
		appModified := false

		for _, t := range transports {
			switch v := t.(type) {
			case string:
				// Old format: convert string to object
				newTransports = append(newTransports, map[string]any{
					"name": v,
				})
				appModified = true

			case map[string]any:
				// Already new format
				newTransports = append(newTransports, v)

			default:
				// Unknown format, keep as is
				newTransports = append(newTransports, t)
			}
		}

		if appModified {
			app["transport"] = newTransports
			apps[i] = app
			modified = true

			appName, _ := app["name"].(string)
			warnings = append(warnings, DeprecationWarning{
				Feature:       "transport string array format",
				Description:   fmt.Sprintf(deprecationDescTransportFormat, appName),
				RemovalVer:    RemovalVersionTransportStringArray,
				MigrationHint: "Run 'go-project-starter migrate' to auto-migrate",
			})
		}
	}

	if modified {
		config["applications"] = apps
	}

	return modified, warnings
}

func (m *Migrator) printMigrationPlan(config map[string]any) {
	fmt.Println("\n=== Migration Plan ===")

	newData, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("Error generating preview: %v\n", err)
		return
	}

	fmt.Println("\nMigrated config would be:")
	fmt.Println("---")
	fmt.Println(string(newData))
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

	// Check transport format
	apps, ok := config["applications"].([]any)
	if !ok {
		return warnings, nil
	}

	for _, appRaw := range apps {
		app, ok := appRaw.(map[string]any)
		if !ok {
			continue
		}

		transports, ok := app["transport"].([]any)
		if !ok {
			continue
		}

		for _, t := range transports {
			if _, isString := t.(string); isString {
				appName, _ := app["name"].(string)
				warnings = append(warnings, DeprecationWarning{
					Feature:       "transport string array format",
					Description:   fmt.Sprintf(deprecationDescTransportFormat, appName),
					RemovalVer:    RemovalVersionTransportStringArray,
					MigrationHint: "Run 'go-project-starter migrate' to auto-migrate",
				})

				break // One warning per app is enough
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
