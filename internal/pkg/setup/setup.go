package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Educentr/go-project-starter/internal/pkg/config"
)

// Command represents available setup subcommands
type Command string

// Setup handles the setup command logic
type Setup struct {
	ConfigDir     string
	TargetDir     string
	SetupConfig   *SetupConfig
	ProjectConfig config.Config
	DryRun        bool
}

// Options for creating a new Setup instance
type Options struct {
	ConfigDir string
	TargetDir string
	DryRun    bool
}

const (
	CommandAll    Command = ""       // Full wizard
	CommandCI     Command = "ci"     // CI/CD setup only
	CommandServer Command = "server" // Server setup only
	CommandDeploy Command = "deploy" // Deploy script generation only

	ciProviderGitHub         = "github"
	ciProviderGitLab         = "gitlab"
	warnNoApplicationsInYAML = "No applications found in project.yaml"
)

var (
	errUnknownCommand    = errors.New("unknown command")
	errUnknownCIProvider = errors.New("unknown CI provider")
)

// New creates a new Setup instance
func New(opts Options) (*Setup, error) {
	// Determine config directory
	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = ".project-config"
	}

	if !filepath.IsAbs(configDir) {
		configDir = filepath.Join(opts.TargetDir, configDir)
	}

	// Load setup config (or defaults)
	setupCfg, err := LoadConfig(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load setup config: %w", err)
	}

	// Load project config for application info
	projectCfg, err := config.GetConfig(configDir, "project.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	return &Setup{
		ConfigDir:     configDir,
		TargetDir:     opts.TargetDir,
		SetupConfig:   setupCfg,
		ProjectConfig: projectCfg,
		DryRun:        opts.DryRun,
	}, nil
}

// Run executes the setup command
func (s *Setup) Run(cmd Command) error {
	switch cmd {
	case CommandAll:
		return s.runFullWizard()
	case CommandCI:
		return s.runCISetup()
	case CommandServer:
		return s.runServerSetup()
	case CommandDeploy:
		return s.runDeploySetup()
	default:
		return fmt.Errorf("%w: %s", errUnknownCommand, cmd)
	}
}

// runFullWizard runs the complete setup wizard
func (s *Setup) runFullWizard() error {
	fmt.Println("=== Go Project Starter Setup Wizard ===")
	fmt.Println()

	// Step 1: Collect configuration via wizard
	if err := s.runWizard(); err != nil {
		return fmt.Errorf("wizard failed: %w", err)
	}

	// Step 2: Save configuration
	if err := s.SetupConfig.SaveConfig(s.ConfigDir); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Println("✓ Configuration saved to setup.yaml")

	// Step 3: Run CI setup
	fmt.Println("\n--- CI/CD Setup ---")
	if err := s.runCISetup(); err != nil {
		return fmt.Errorf("CI setup failed: %w", err)
	}

	// Step 4: Run server setup
	fmt.Println("\n--- Server Setup ---")
	if err := s.runServerSetup(); err != nil {
		return fmt.Errorf("server setup failed: %w", err)
	}

	// Step 5: Generate deploy script
	fmt.Println("\n--- Deploy Script ---")
	if err := s.runDeploySetup(); err != nil {
		return fmt.Errorf("deploy setup failed: %w", err)
	}

	fmt.Println("\n=== Setup Complete ===")
	return nil
}

// runCISetup handles CI/CD configuration
func (s *Setup) runCISetup() error {
	switch s.SetupConfig.CI.Provider {
	case ciProviderGitHub:
		return s.setupGitHub()
	case ciProviderGitLab:
		return s.setupGitLab()
	default:
		return fmt.Errorf("%w: %s", errUnknownCIProvider, s.SetupConfig.CI.Provider)
	}
}

// runServerSetup handles server configuration
func (s *Setup) runServerSetup() error {
	for _, env := range s.SetupConfig.Environments {
		fmt.Printf("\n--- Setting up %s environment (%s) ---\n", env.Name, env.Server.Host)
		if err := s.setupServer(env); err != nil {
			return fmt.Errorf("failed to setup %s: %w", env.Name, err)
		}
	}
	return nil
}

// runDeploySetup generates deploy.sh script
func (s *Setup) runDeploySetup() error {
	return s.generateDeployScript()
}

// GetProjectName returns the project name from project config
func (s *Setup) GetProjectName() string {
	return s.ProjectConfig.Main.Name
}

// GetApplications returns application names from project config
func (s *Setup) GetApplications() []string {
	var apps []string
	for _, app := range s.ProjectConfig.Applications {
		apps = append(apps, app.Name)
	}
	return apps
}

// GetRegistryType returns registry type from project config
func (s *Setup) GetRegistryType() string {
	return s.ProjectConfig.Main.RegistryType
}

// PrintManualInstructions prints commands for manual execution
func PrintManualInstructions(title string, commands []string) {
	fmt.Printf("\n%s\n", title)
	fmt.Println("Please run the following commands manually:")
	fmt.Println("```bash")
	for _, cmd := range commands {
		fmt.Println(cmd)
	}
	fmt.Println("```")
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	fmt.Printf("✓ %s\n", message)
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	fmt.Printf("⚠ %s\n", message)
}

// PrintError prints an error message
func PrintError(message string) {
	fmt.Fprintf(os.Stderr, "✗ %s\n", message)
}

// PrintInfo prints an info message
func PrintInfo(message string) {
	fmt.Printf("ℹ %s\n", message)
}
