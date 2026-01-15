package setup

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SetupConfig represents the setup.yaml configuration
//
//nolint:revive // using SetupConfig name for clarity in external usage
type SetupConfig struct {
	AdminEmail    string              `yaml:"admin_email"`
	CI            CIConfig            `yaml:"ci"`
	Environments  []EnvironmentConfig `yaml:"environments"`
	Registry      RegistryConfig      `yaml:"registry"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

// CIConfig represents CI/CD provider configuration
type CIConfig struct {
	Provider string `yaml:"provider"` // github or gitlab
	Repo     string `yaml:"repo"`     // owner/repo for GitHub, project path for GitLab
}

// EnvironmentConfig represents a single environment (staging, production)
type EnvironmentConfig struct {
	Name           string           `yaml:"name"`   // staging, production
	Branch         string           `yaml:"branch"` // staging, main
	Server         ServerConfig     `yaml:"server"`
	Services       []ServiceConfig  `yaml:"services"`
	OnlineConf     OnlineConfConfig `yaml:"onlineconf"`
	InternalSubnet string           `yaml:"internal_subnet"`
}

// ServerConfig represents server connection details
type ServerConfig struct {
	Host       string `yaml:"host"`
	User       string `yaml:"user"`        // root for initial setup
	DeployUser string `yaml:"deploy_user"` // deploy
	Port       int    `yaml:"port"`        // SSH port, default 22
}

// ServiceConfig represents a service configuration
type ServiceConfig struct {
	Name       string `yaml:"name"`
	Domain     string `yaml:"domain,omitempty"` // empty if not public
	PortPrefix int    `yaml:"port_prefix"`
}

// OnlineConfConfig represents OnlineConf connection details
type OnlineConfConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// RegistryConfig represents Docker registry configuration
type RegistryConfig struct {
	Type      string `yaml:"type"`      // github, digitalocean, aws, selfhosted
	Server    string `yaml:"server"`    // ghcr.io, registry.digitalocean.com, etc.
	Container string `yaml:"container"` // owner/project
	// Credentials (not saved to yaml, asked interactively)
	Username string `yaml:"-"`
	Password string `yaml:"-"`
	Token    string `yaml:"-"`
}

// NotificationsConfig represents notification settings
type NotificationsConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Slack    SlackConfig    `yaml:"slack"`
}

// TelegramConfig represents Telegram bot configuration
type TelegramConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"bot_token,omitempty"`
	ChatID   string `yaml:"chat_id,omitempty"`
}

// SlackConfig represents Slack webhook configuration
type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url,omitempty"`
}

const setupConfigFileName = "setup.yaml"

// DefaultSetupConfig returns a new SetupConfig with default values
func DefaultSetupConfig() *SetupConfig {
	return &SetupConfig{
		CI: CIConfig{
			Provider: "github",
		},
		Registry: RegistryConfig{
			Type:   "github",
			Server: "ghcr.io",
		},
		Notifications: NotificationsConfig{
			Telegram: TelegramConfig{Enabled: true},
			Slack:    SlackConfig{Enabled: false},
		},
	}
}

// LoadConfig loads setup.yaml from the given directory
func LoadConfig(configDir string) (*SetupConfig, error) {
	configPath := filepath.Join(configDir, setupConfigFileName)

	// If file doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultSetupConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := DefaultSetupConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves the configuration to setup.yaml
func (c *SetupConfig) SaveConfig(configDir string) error {
	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, setupConfigFileName)

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// HasPublicServices returns true if any environment has public services
func (c *SetupConfig) HasPublicServices() bool {
	for _, env := range c.Environments {
		for _, svc := range env.Services {
			if svc.Domain != "" {
				return true
			}
		}
	}
	return false
}

// GetPublicServices returns all public services across all environments
func (c *SetupConfig) GetPublicServices() []struct {
	Environment string
	Service     ServiceConfig
} {
	var result []struct {
		Environment string
		Service     ServiceConfig
	}

	for _, env := range c.Environments {
		for _, svc := range env.Services {
			if svc.Domain != "" {
				result = append(result, struct {
					Environment string
					Service     ServiceConfig
				}{
					Environment: env.Name,
					Service:     svc,
				})
			}
		}
	}

	return result
}
