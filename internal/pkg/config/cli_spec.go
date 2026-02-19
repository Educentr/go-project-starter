package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// CLISpecFlag represents a single flag for a CLI command
type CLISpecFlag struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
	Description string `yaml:"description"`
}

// CLISpecSubcommand represents a subcommand within a CLI command
type CLISpecSubcommand struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Flags       []CLISpecFlag `yaml:"flags"`
}

// CLISpecCommand represents a top-level CLI command
type CLISpecCommand struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Subcommands []CLISpecSubcommand `yaml:"subcommands"`
	Flags       []CLISpecFlag       `yaml:"flags"`
}

// CLISpec represents the full CLI commands specification
type CLISpec struct {
	Commands []CLISpecCommand `yaml:"commands"`
}

// validFlagTypes contains the allowed flag types
var validFlagTypes = map[string]bool{
	"string":   true,
	"int":      true,
	"bool":     true,
	"float64":  true,
	"duration": true,
}

// ParseCLISpec reads and parses a CLI spec YAML file
func ParseCLISpec(path string) (*CLISpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read CLI spec file: %s", path)
	}

	var spec CLISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errors.Wrapf(err, "failed to parse CLI spec file: %s", path)
	}

	if err := spec.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid CLI spec: %s", path)
	}

	return &spec, nil
}

// Validate checks the CLI spec for errors
func (s *CLISpec) Validate() error {
	if len(s.Commands) == 0 {
		return errors.New("no commands defined")
	}

	seenCommands := make(map[string]struct{})

	for _, cmd := range s.Commands {
		if cmd.Name == "" {
			return errors.New("command name is empty")
		}

		if _, exists := seenCommands[cmd.Name]; exists {
			return fmt.Errorf("duplicate command name: %s", cmd.Name)
		}
		seenCommands[cmd.Name] = struct{}{}

		// Command with subcommands cannot also have flags at the top level
		if len(cmd.Subcommands) > 0 && len(cmd.Flags) > 0 {
			return fmt.Errorf("command '%s' has both subcommands and flags; flags should be on subcommands", cmd.Name)
		}

		// Validate flags on leaf commands
		if len(cmd.Subcommands) == 0 {
			if err := validateFlags(cmd.Flags); err != nil {
				return errors.Wrapf(err, "command '%s'", cmd.Name)
			}
		}

		// Validate subcommands
		seenSubs := make(map[string]struct{})
		for _, sub := range cmd.Subcommands {
			if sub.Name == "" {
				return fmt.Errorf("command '%s' has a subcommand with empty name", cmd.Name)
			}

			if _, exists := seenSubs[sub.Name]; exists {
				return fmt.Errorf("command '%s': duplicate subcommand name: %s", cmd.Name, sub.Name)
			}
			seenSubs[sub.Name] = struct{}{}

			if err := validateFlags(sub.Flags); err != nil {
				return errors.Wrapf(err, "command '%s %s'", cmd.Name, sub.Name)
			}
		}
	}

	return nil
}

func validateFlags(flags []CLISpecFlag) error {
	seenFlags := make(map[string]struct{})

	for _, f := range flags {
		if f.Name == "" {
			return errors.New("flag name is empty")
		}

		if _, exists := seenFlags[f.Name]; exists {
			return fmt.Errorf("duplicate flag name: %s", f.Name)
		}
		seenFlags[f.Name] = struct{}{}

		flagType := f.Type
		if flagType == "" {
			flagType = "string" // default
		}

		if !validFlagTypes[flagType] {
			return fmt.Errorf("flag '%s': invalid type '%s' (valid: string, int, bool, float64, duration)", f.Name, flagType)
		}
	}

	return nil
}

// toPascalCase converts a snake_case or kebab-case string to PascalCase
func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})

	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]))
			result.WriteString(part[1:])
		}
	}

	return result.String()
}
