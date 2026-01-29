package init

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"gopkg.in/yaml.v3"
)

// ProjectConfig represents a minimal project.yaml structure
type ProjectConfig struct {
	Main         MainConfig          `yaml:"main"`
	PostGenerate []string            `yaml:"post_generate"`
	Git          GitConfig           `yaml:"git"`
	Tools        ToolsConfig         `yaml:"tools"`
	Rest         []RestConfig        `yaml:"rest,omitempty"`
	Worker       []WorkerConfig      `yaml:"worker,omitempty"`
	Driver       []DriverConfig      `yaml:"driver,omitempty"`
	Applications []ApplicationConfig `yaml:"applications"`
}

// MainConfig represents main project settings
type MainConfig struct {
	Name         string `yaml:"name"`
	Logger       string `yaml:"logger"`
	RegistryType string `yaml:"registry_type"`
}

// GitConfig represents git repository settings
type GitConfig struct {
	Repo       string `yaml:"repo"`
	ModulePath string `yaml:"module_path"`
}

// ToolsConfig represents tool versions
type ToolsConfig struct {
	ProtobufVersion string `yaml:"protobuf_version"`
	GolangVersion   string `yaml:"golang_version"`
	OgenVersion     string `yaml:"ogen_version"`
	GolangciVersion string `yaml:"golangci_version"`
}

// RestConfig represents REST API configuration
type RestConfig struct {
	Name              string `yaml:"name"`
	Port              int    `yaml:"port"`
	Version           string `yaml:"version"`
	GeneratorType     string `yaml:"generator_type"`
	GeneratorTemplate string `yaml:"generator_template,omitempty"`
}

// WorkerConfig represents background worker configuration
type WorkerConfig struct {
	Name              string `yaml:"name"`
	GeneratorType     string `yaml:"generator_type"`
	GeneratorTemplate string `yaml:"generator_template"`
}

// DriverConfig represents external driver configuration
type DriverConfig struct {
	Name             string `yaml:"name"`
	Import           string `yaml:"import"`
	Package          string `yaml:"package"`
	ObjName          string `yaml:"obj_name"`
	ServiceInjection string `yaml:"service_injection,omitempty"`
}

// ApplicationConfig represents application configuration
type ApplicationConfig struct {
	Name      string                    `yaml:"name"`
	Transport []string                  `yaml:"transport,omitempty"`
	Worker    []string                  `yaml:"worker,omitempty"`
	Driver    []ApplicationDriverConfig `yaml:"driver,omitempty"`
}

// ApplicationDriverConfig represents driver usage in application
type ApplicationDriverConfig struct {
	Name   string   `yaml:"name"`
	Params []string `yaml:"params,omitempty"`
}

// Init handles the project initialization
type Init struct {
	ConfigDir string
	TargetDir string
	Config    *ProjectConfig
}

// Options for creating Init instance
type Options struct {
	ConfigDir string
	TargetDir string
}

const (
	defaultGolangVersion   = "1.24"
	defaultGolangciVersion = "1.64.8"
	defaultProtobufVersion = "1.7.0"
	defaultOgenVersion     = "v1.18.0"
	defaultSysPort         = 8085
	defaultAPIPort         = 8080
	defaultRegistryType    = "github"
	defaultLogger          = "zerolog"
	gitURLPartsCount       = 2
	gitSSHPrefix           = "git@"
	gitHTTPSPrefix         = "https://"
	projectConfigFile      = "project.yaml"
	dirPermissions         = 0755
	filePermissions        = 0600
)

// New creates a new Init instance
func New(opts Options) *Init {
	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = ".project-config"
	}

	if !filepath.IsAbs(configDir) {
		configDir = filepath.Join(opts.TargetDir, configDir)
	}

	return &Init{
		ConfigDir: configDir,
		TargetDir: opts.TargetDir,
		Config:    &ProjectConfig{},
	}
}

// Run executes the init wizard
func (i *Init) Run() error {
	fmt.Println("=== Go Project Starter - Project Initialization ===")
	fmt.Println()

	// Step 1: Basic project info
	if err := i.askBasicInfo(); err != nil {
		return err
	}

	// Step 2: Git configuration
	if err := i.askGitConfig(); err != nil {
		return err
	}

	// Step 3: Project type selection
	if err := i.askProjectType(); err != nil {
		return err
	}

	// Step 4: Set defaults
	i.setDefaults()

	// Step 5: Save configuration
	if err := i.saveConfig(); err != nil {
		return err
	}

	fmt.Println("\nConfiguration saved to", filepath.Join(i.ConfigDir, projectConfigFile))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review and customize project.yaml")
	fmt.Println("  2. Run: go-project-starter --configDir=" + i.ConfigDir + " --target=" + i.TargetDir)
	fmt.Println("  3. Run: go-project-starter setup --configDir=" + i.ConfigDir + " --target=" + i.TargetDir)

	return nil
}

func (i *Init) askBasicInfo() error {
	fmt.Println("--- Basic Information ---")

	// Project name
	namePrompt := &survey.Input{
		Message: "Project name (lowercase, no spaces):",
	}

	if err := survey.AskOne(namePrompt, &i.Config.Main.Name, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Normalize name
	i.Config.Main.Name = strings.ToLower(strings.ReplaceAll(i.Config.Main.Name, " ", "-"))

	// Logger
	loggerPrompt := &survey.Select{
		Message: "Logger:",
		Options: []string{"zerolog", "zap", "logrus"},
		Default: defaultLogger,
	}

	if err := survey.AskOne(loggerPrompt, &i.Config.Main.Logger); err != nil {
		return err
	}

	// Registry type
	registryPrompt := &survey.Select{
		Message: "Docker registry type:",
		Options: []string{"github", "digitalocean", "aws", "selfhosted"},
		Default: defaultRegistryType,
	}

	return survey.AskOne(registryPrompt, &i.Config.Main.RegistryType)
}

func (i *Init) askGitConfig() error {
	fmt.Println("\n--- Git Configuration ---")

	// Git URL
	gitPrompt := &survey.Input{
		Message: "Git repository URL (e.g., git@github.com:owner/repo.git):",
	}

	if err := survey.AskOne(gitPrompt, &i.Config.Git.Repo, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Auto-derive module path from git URL
	modulePath := deriveModulePath(i.Config.Git.Repo)

	modulePrompt := &survey.Input{
		Message: "Go module path:",
		Default: modulePath,
	}

	return survey.AskOne(modulePrompt, &i.Config.Git.ModulePath, survey.WithValidator(survey.Required))
}

func (i *Init) askProjectType() error {
	fmt.Println("\n--- Project Type ---")

	var projectType string

	typePrompt := &survey.Select{
		Message: "What type of project?",
		Options: []string{
			"REST API (OpenAPI)",
			"Telegram Bot",
			"gRPC Service",
			"Background Worker",
			"Custom (manual configuration)",
		},
	}

	if err := survey.AskOne(typePrompt, &projectType); err != nil {
		return err
	}

	switch projectType {
	case "REST API (OpenAPI)":
		return i.configureRESTAPI()
	case "Telegram Bot":
		return i.configureTelegramBot()
	case "gRPC Service":
		return i.configureGRPC()
	case "Background Worker":
		return i.configureWorker()
	default:
		// Custom - just create minimal config
		i.createMinimalApp()
	}

	return nil
}

func (i *Init) configureRESTAPI() error {
	fmt.Println("\n--- REST API Configuration ---")

	// Ask for API name
	var apiName string

	namePrompt := &survey.Input{
		Message: "API name (e.g., api, public, admin):",
		Default: "api",
	}

	if err := survey.AskOne(namePrompt, &apiName); err != nil {
		return err
	}

	// Ask for OpenAPI spec path - just for documentation, not actually used
	var specPath string

	specPrompt := &survey.Input{
		Message: "OpenAPI spec file path:",
		Default: "api/openapi.yaml",
	}

	if err := survey.AskOne(specPrompt, &specPath); err != nil {
		return err
	}

	// Add REST config
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:          apiName,
		Port:          defaultAPIPort,
		Version:       "v1",
		GeneratorType: "ogen",
	})

	// Add sys endpoint
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:              "sys",
		Port:              defaultSysPort,
		Version:           "v1",
		GeneratorType:     "template",
		GeneratorTemplate: "sys",
	})

	// Create application
	i.Config.Applications = append(i.Config.Applications, ApplicationConfig{
		Name:      i.Config.Main.Name,
		Transport: []string{apiName, "sys"},
	})

	return nil
}

func (i *Init) configureTelegramBot() error {
	fmt.Println("\n--- Telegram Bot Configuration ---")

	// Application name
	var appName string

	namePrompt := &survey.Input{
		Message: "Bot application name:",
		Default: "bot",
	}

	if err := survey.AskOne(namePrompt, &appName); err != nil {
		return err
	}

	// Add sys endpoint for metrics
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:              "sys",
		Port:              defaultSysPort,
		Version:           "v1",
		GeneratorType:     "template",
		GeneratorTemplate: "sys",
	})

	// Add telegram worker
	i.Config.Worker = append(i.Config.Worker, WorkerConfig{
		Name:              "telegrambot",
		GeneratorType:     "template",
		GeneratorTemplate: "telegram",
	})

	// Add telegram driver
	i.Config.Driver = append(i.Config.Driver, DriverConfig{
		Name:    "telegram",
		Import:  fmt.Sprintf("%s/pkg/drivers/telegram", i.Config.Git.ModulePath),
		Package: "telegram",
		ObjName: "Telegram",
		ServiceInjection: `telegram.BaseAuth
telegram.UnimplementedPayment`,
	})

	// Create application
	i.Config.Applications = append(i.Config.Applications, ApplicationConfig{
		Name:      appName,
		Transport: []string{"sys"},
		Worker:    []string{"telegrambot"},
		Driver: []ApplicationDriverConfig{
			{
				Name:   "telegram",
				Params: []string{"WithUpdatePoller()"},
			},
		},
	})

	return nil
}

func (i *Init) configureGRPC() error {
	fmt.Println("\n--- gRPC Service Configuration ---")

	var serviceName string

	namePrompt := &survey.Input{
		Message: "Service name:",
		Default: "service",
	}

	if err := survey.AskOne(namePrompt, &serviceName); err != nil {
		return err
	}

	// Add sys endpoint
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:              "sys",
		Port:              defaultSysPort,
		Version:           "v1",
		GeneratorType:     "template",
		GeneratorTemplate: "sys",
	})

	// Create application
	i.Config.Applications = append(i.Config.Applications, ApplicationConfig{
		Name:      serviceName,
		Transport: []string{"sys"},
	})

	fmt.Println("\nNote: Add your gRPC configuration manually to project.yaml")

	return nil
}

func (i *Init) configureWorker() error {
	fmt.Println("\n--- Background Worker Configuration ---")

	var workerName string

	namePrompt := &survey.Input{
		Message: "Worker name:",
		Default: "worker",
	}

	if err := survey.AskOne(namePrompt, &workerName); err != nil {
		return err
	}

	// Add sys endpoint
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:              "sys",
		Port:              defaultSysPort,
		Version:           "v1",
		GeneratorType:     "template",
		GeneratorTemplate: "sys",
	})

	// Add daemon worker
	i.Config.Worker = append(i.Config.Worker, WorkerConfig{
		Name:              workerName,
		GeneratorType:     "template",
		GeneratorTemplate: "daemon",
	})

	// Create application
	i.Config.Applications = append(i.Config.Applications, ApplicationConfig{
		Name:      workerName,
		Transport: []string{"sys"},
		Worker:    []string{workerName},
	})

	return nil
}

func (i *Init) createMinimalApp() {
	// Add sys endpoint
	i.Config.Rest = append(i.Config.Rest, RestConfig{
		Name:              "sys",
		Port:              defaultSysPort,
		Version:           "v1",
		GeneratorType:     "template",
		GeneratorTemplate: "sys",
	})

	// Create minimal application
	i.Config.Applications = append(i.Config.Applications, ApplicationConfig{
		Name:      i.Config.Main.Name,
		Transport: []string{"sys"},
	})
}

func (i *Init) setDefaults() {
	i.Config.PostGenerate = []string{}
	i.Config.Tools = ToolsConfig{
		ProtobufVersion: defaultProtobufVersion,
		GolangVersion:   defaultGolangVersion,
		OgenVersion:     defaultOgenVersion,
		GolangciVersion: defaultGolangciVersion,
	}
}

func (i *Init) saveConfig() error {
	// Ensure config directory exists
	if err := os.MkdirAll(i.ConfigDir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(i.ConfigDir, projectConfigFile)

	data, err := yaml.Marshal(i.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, filePermissions); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// deriveModulePath derives Go module path from git URL
func deriveModulePath(gitURL string) string {
	gitURL = strings.TrimSuffix(gitURL, ".git")

	if strings.HasPrefix(gitURL, gitSSHPrefix) {
		// git@github.com:owner/repo -> github.com/owner/repo
		parts := strings.SplitN(gitURL[len(gitSSHPrefix):], ":", gitURLPartsCount)
		if len(parts) == gitURLPartsCount {
			return parts[0] + "/" + parts[1]
		}
	}

	if strings.HasPrefix(gitURL, gitHTTPSPrefix) {
		// https://github.com/owner/repo -> github.com/owner/repo
		return gitURL[len(gitHTTPSPrefix):]
	}

	return gitURL
}
