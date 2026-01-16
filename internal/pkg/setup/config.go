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
	Registry      RegistryConfig      `yaml:"registry"`
	Servers       []ServerConfig      `yaml:"servers"`
	Applications  []ApplicationConfig `yaml:"applications"`
	Environments  []EnvironmentConfig `yaml:"environments"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

// CIConfig represents CI/CD provider configuration
type CIConfig struct {
	Provider string `yaml:"provider"` // github or gitlab
	Repo     string `yaml:"repo"`     // owner/repo for GitHub, project path for GitLab
}

// RegistryConfig represents Docker registry configuration
type RegistryConfig struct {
	Type      string `yaml:"type"`      // github, digitalocean, aws, selfhosted
	Server    string `yaml:"server"`    // ghcr.io, registry.digitalocean.com, etc.
	Container string `yaml:"container"` // owner/project
}

// ServerConfig represents a deployment server
type ServerConfig struct {
	Name       string `yaml:"name"`        // logical name: prod-1, staging, etc.
	Host       string `yaml:"host"`        // IP or hostname
	SSHPort    int    `yaml:"ssh_port"`    // default 22
	SSHUser    string `yaml:"ssh_user"`    // root for initial setup
	DeployUser string `yaml:"deploy_user"` // deploy user for CI/CD
}

// ApplicationConfig represents an application from project.yaml
type ApplicationConfig struct {
	Name       string `yaml:"name"`
	Singleton  bool   `yaml:"singleton"`        // true if can only run one instance (e.g., telegram poller)
	PortPrefix int    `yaml:"port_prefix"`      // base port prefix for this app
	Domain     string `yaml:"domain,omitempty"` // public domain if applicable
}

// EnvironmentConfig represents a deployment environment (production, staging)
type EnvironmentConfig struct {
	Name           string             `yaml:"name"`   // production, staging
	Branch         string             `yaml:"branch"` // main, staging
	OnlineConf     OnlineConfConfig   `yaml:"onlineconf"`
	InternalSubnet string             `yaml:"internal_subnet"`
	Deployments    []DeploymentConfig `yaml:"deployments"`
}

// DeploymentConfig represents which apps run on which server
type DeploymentConfig struct {
	Server string   `yaml:"server"` // server name reference
	Apps   []string `yaml:"apps"`   // application names to deploy
}

// OnlineConfConfig represents OnlineConf connection details
type OnlineConfConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
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

// SingletonViolationError represents an error when singleton app is deployed to multiple servers
type SingletonViolationError struct {
	App         string
	Environment string
	Server1     string
	Server2     string
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
			Telegram: TelegramConfig{Enabled: false},
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

// GetServerByName returns server config by name
func (c *SetupConfig) GetServerByName(name string) *ServerConfig {
	for i := range c.Servers {
		if c.Servers[i].Name == name {
			return &c.Servers[i]
		}
	}

	return nil
}

// GetApplicationByName returns application config by name
func (c *SetupConfig) GetApplicationByName(name string) *ApplicationConfig {
	for i := range c.Applications {
		if c.Applications[i].Name == name {
			return &c.Applications[i]
		}
	}

	return nil
}

// GetEnvironmentByBranch returns environment config by branch name
func (c *SetupConfig) GetEnvironmentByBranch(branch string) *EnvironmentConfig {
	for i := range c.Environments {
		if c.Environments[i].Branch == branch {
			return &c.Environments[i]
		}
	}

	return nil
}

// HasPublicServices returns true if any application has a public domain
func (c *SetupConfig) HasPublicServices() bool {
	for _, app := range c.Applications {
		if app.Domain != "" {
			return true
		}
	}
	return false
}

// GetSingletonApps returns list of singleton application names
func (c *SetupConfig) GetSingletonApps() []string {
	var result []string

	for _, app := range c.Applications {
		if app.Singleton {
			result = append(result, app.Name)
		}
	}

	return result
}

// ValidateDeployments checks that singleton apps are not deployed to multiple servers in same environment
func (c *SetupConfig) ValidateDeployments() error {
	singletons := make(map[string]bool)

	for _, app := range c.Applications {
		if app.Singleton {
			singletons[app.Name] = true
		}
	}

	for _, env := range c.Environments {
		// Track which singleton apps are deployed in this environment
		singletonServers := make(map[string]string) // app -> server

		for _, deployment := range env.Deployments {
			for _, appName := range deployment.Apps {
				if singletons[appName] {
					if existingServer, exists := singletonServers[appName]; exists {
						return &SingletonViolationError{
							App:         appName,
							Environment: env.Name,
							Server1:     existingServer,
							Server2:     deployment.Server,
						}
					}

					singletonServers[appName] = deployment.Server
				}
			}
		}
	}

	return nil
}

func (e *SingletonViolationError) Error() string {
	return "singleton app '" + e.App + "' cannot be deployed to multiple servers (" +
		e.Server1 + ", " + e.Server2 + ") in environment '" + e.Environment + "'"
}
