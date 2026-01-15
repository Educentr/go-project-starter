package setup

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// runWizard runs the interactive setup wizard
func (s *Setup) runWizard() error {
	// Step 1: Admin email
	if err := s.askAdminEmail(); err != nil {
		return err
	}

	// Step 2: CI/CD provider
	if err := s.askCIProvider(); err != nil {
		return err
	}

	// Step 3: Repository
	if err := s.askRepository(); err != nil {
		return err
	}

	// Step 4: Registry configuration
	if err := s.askRegistry(); err != nil {
		return err
	}

	// Step 5: Environments
	if err := s.askEnvironments(); err != nil {
		return err
	}

	// Step 6: Notifications
	if err := s.askNotifications(); err != nil {
		return err
	}

	return nil
}

func (s *Setup) askAdminEmail() error {
	prompt := &survey.Input{
		Message: "Admin email (for certbot and notifications):",
		Default: s.SetupConfig.AdminEmail,
	}
	return survey.AskOne(prompt, &s.SetupConfig.AdminEmail, survey.WithValidator(survey.Required))
}

func (s *Setup) askCIProvider() error {
	options := []string{"github", "gitlab"}
	defaultIdx := 0
	for i, opt := range options {
		if opt == s.SetupConfig.CI.Provider {
			defaultIdx = i
			break
		}
	}

	prompt := &survey.Select{
		Message: "CI/CD provider:",
		Options: options,
		Default: options[defaultIdx],
	}
	return survey.AskOne(prompt, &s.SetupConfig.CI.Provider)
}

func (s *Setup) askRepository() error {
	message := "Repository (owner/repo):"
	if s.SetupConfig.CI.Provider == "gitlab" {
		message = "Repository (group/project or full path):"
	}

	prompt := &survey.Input{
		Message: message,
		Default: s.SetupConfig.CI.Repo,
	}
	return survey.AskOne(prompt, &s.SetupConfig.CI.Repo, survey.WithValidator(survey.Required))
}

func (s *Setup) askRegistry() error {
	// Registry type from project.yaml takes precedence
	registryType := s.GetRegistryType()
	if registryType != "" {
		s.SetupConfig.Registry.Type = registryType
		PrintInfo(fmt.Sprintf("Using registry type from project.yaml: %s", registryType))
	} else {
		options := []string{"github", "digitalocean", "aws", "selfhosted"}
		prompt := &survey.Select{
			Message: "Docker registry type:",
			Options: options,
			Default: s.SetupConfig.Registry.Type,
		}
		if err := survey.AskOne(prompt, &s.SetupConfig.Registry.Type); err != nil {
			return err
		}
	}

	// Registry server
	defaultServer := ""
	switch s.SetupConfig.Registry.Type {
	case "github":
		defaultServer = "ghcr.io"
	case "digitalocean":
		defaultServer = "registry.digitalocean.com"
	case "aws":
		defaultServer = "<account>.dkr.ecr.<region>.amazonaws.com"
	}

	if s.SetupConfig.Registry.Server == "" {
		s.SetupConfig.Registry.Server = defaultServer
	}

	prompt := &survey.Input{
		Message: "Registry server:",
		Default: s.SetupConfig.Registry.Server,
	}
	if err := survey.AskOne(prompt, &s.SetupConfig.Registry.Server); err != nil {
		return err
	}

	// Container path
	prompt = &survey.Input{
		Message: "Container path (e.g., owner/project):",
		Default: s.SetupConfig.Registry.Container,
	}
	return survey.AskOne(prompt, &s.SetupConfig.Registry.Container, survey.WithValidator(survey.Required))
}

func (s *Setup) askEnvironments() error {
	// Ask how many environments
	var envCount string
	if len(s.SetupConfig.Environments) > 0 {
		envCount = strconv.Itoa(len(s.SetupConfig.Environments))
	} else {
		envCount = "1"
	}

	prompt := &survey.Select{
		Message: "How many environments do you need?",
		Options: []string{"1", "2"},
		Default: envCount,
	}
	if err := survey.AskOne(prompt, &envCount); err != nil {
		return err
	}

	count, err := strconv.Atoi(envCount)
	if err != nil {
		count = 1
	}

	// Ensure we have enough environment slots
	for len(s.SetupConfig.Environments) < count {
		s.SetupConfig.Environments = append(s.SetupConfig.Environments, EnvironmentConfig{
			Server: ServerConfig{
				Port:       22,
				DeployUser: "deploy",
			},
		})
	}

	// Trim if needed
	if len(s.SetupConfig.Environments) > count {
		s.SetupConfig.Environments = s.SetupConfig.Environments[:count]
	}

	// Configure each environment
	defaultNames := []string{"production", "staging"}
	defaultBranches := []string{"main", "staging"}

	for i := 0; i < count; i++ {
		fmt.Printf("\n--- Environment %d ---\n", i+1)

		env := &s.SetupConfig.Environments[i]

		// Name
		defaultName := ""
		if i < len(defaultNames) {
			defaultName = defaultNames[i]
		}
		if env.Name == "" {
			env.Name = defaultName
		}

		namePrompt := &survey.Input{
			Message: "Environment name:",
			Default: env.Name,
		}
		if err := survey.AskOne(namePrompt, &env.Name, survey.WithValidator(survey.Required)); err != nil {
			return err
		}

		// Branch
		defaultBranch := ""
		if i < len(defaultBranches) {
			defaultBranch = defaultBranches[i]
		}
		if env.Branch == "" {
			env.Branch = defaultBranch
		}

		branchPrompt := &survey.Input{
			Message: "Git branch:",
			Default: env.Branch,
		}
		if err := survey.AskOne(branchPrompt, &env.Branch, survey.WithValidator(survey.Required)); err != nil {
			return err
		}

		// Server
		if err := s.askServerConfig(env); err != nil {
			return err
		}

		// Services
		if err := s.askServices(env); err != nil {
			return err
		}

		// OnlineConf
		if err := s.askOnlineConf(env); err != nil {
			return err
		}

		// Internal subnet
		if env.InternalSubnet == "" {
			env.InternalSubnet = "10.0.0.0/8"
		}
		subnetPrompt := &survey.Input{
			Message: "Internal subnet:",
			Default: env.InternalSubnet,
		}
		if err := survey.AskOne(subnetPrompt, &env.InternalSubnet); err != nil {
			return err
		}
	}

	return nil
}

func (s *Setup) askServerConfig(env *EnvironmentConfig) error {
	// Host
	hostPrompt := &survey.Input{
		Message: "Server host (IP or hostname):",
		Default: env.Server.Host,
	}
	if err := survey.AskOne(hostPrompt, &env.Server.Host, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// SSH port
	if env.Server.Port == 0 {
		env.Server.Port = 22
	}
	var portStr string
	portPrompt := &survey.Input{
		Message: "SSH port:",
		Default: strconv.Itoa(env.Server.Port),
	}
	if err := survey.AskOne(portPrompt, &portStr); err != nil {
		return err
	}

	if port, err := strconv.Atoi(portStr); err == nil {
		env.Server.Port = port
	}

	// Root user (for initial setup)
	if env.Server.User == "" {
		env.Server.User = "root"
	}
	userPrompt := &survey.Input{
		Message: "SSH user (for initial setup):",
		Default: env.Server.User,
	}
	if err := survey.AskOne(userPrompt, &env.Server.User); err != nil {
		return err
	}

	// Deploy user
	if env.Server.DeployUser == "" {
		env.Server.DeployUser = "deploy"
	}
	deployUserPrompt := &survey.Input{
		Message: "Deploy user (will be created):",
		Default: env.Server.DeployUser,
	}
	return survey.AskOne(deployUserPrompt, &env.Server.DeployUser)
}

func (s *Setup) askServices(env *EnvironmentConfig) error {
	// Get applications from project config
	apps := s.GetApplications()
	if len(apps) == 0 {
		PrintWarning(warnNoApplicationsInYAML)
		return nil
	}

	// Get REST transports with public_service flag from project config
	restTransports := s.getPublicRestTransports()

	// Build services list from applications
	if len(env.Services) == 0 {
		for _, app := range apps {
			svc := ServiceConfig{
				Name:       app,
				PortPrefix: 80,
			}
			env.Services = append(env.Services, svc)
		}
	}

	fmt.Printf("\nConfiguring services for %s:\n", env.Name)

	for i := range env.Services {
		svc := &env.Services[i]

		fmt.Printf("\n  Service: %s\n", svc.Name)

		// Check if this service has public REST transport
		isPublic := false
		for _, t := range restTransports {
			if strings.Contains(t, svc.Name) {
				isPublic = true
				break
			}
		}

		// Domain (only ask if service can be public)
		if isPublic {
			domainPrompt := &survey.Input{
				Message: fmt.Sprintf("  Domain for %s (leave empty if not public):", svc.Name),
				Default: svc.Domain,
			}
			if err := survey.AskOne(domainPrompt, &svc.Domain); err != nil {
				return err
			}
		}

		// Port prefix
		var portStr string
		portPrompt := &survey.Input{
			Message: fmt.Sprintf("  Port prefix for %s:", svc.Name),
			Default: strconv.Itoa(svc.PortPrefix),
		}
		if err := survey.AskOne(portPrompt, &portStr); err != nil {
			return err
		}

		if port, err := strconv.Atoi(portStr); err == nil {
			svc.PortPrefix = port
		}
	}

	return nil
}

func (s *Setup) askOnlineConf(env *EnvironmentConfig) error {
	fmt.Println("\n  OnlineConf configuration:")

	// Host
	hostPrompt := &survey.Input{
		Message: "  OnlineConf host:",
		Default: env.OnlineConf.Host,
	}
	if err := survey.AskOne(hostPrompt, &env.OnlineConf.Host, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Port
	if env.OnlineConf.Port == 0 {
		env.OnlineConf.Port = 443
	}
	var portStr string
	portPrompt := &survey.Input{
		Message: "  OnlineConf port:",
		Default: strconv.Itoa(env.OnlineConf.Port),
	}
	if err := survey.AskOne(portPrompt, &portStr); err != nil {
		return err
	}

	if port, err := strconv.Atoi(portStr); err == nil {
		env.OnlineConf.Port = port
	}

	// User (optional, for display only - actual credentials go to CI)
	userPrompt := &survey.Input{
		Message: "  OnlineConf user (for reference):",
		Default: env.OnlineConf.User,
	}
	if err := survey.AskOne(userPrompt, &env.OnlineConf.User); err != nil {
		return err
	}

	return nil
}

func (s *Setup) askNotifications() error {
	fmt.Println("\n--- Notifications ---")

	// Telegram
	var enableTelegram bool
	telegramPrompt := &survey.Confirm{
		Message: "Enable Telegram notifications?",
		Default: s.SetupConfig.Notifications.Telegram.Enabled,
	}
	if err := survey.AskOne(telegramPrompt, &enableTelegram); err != nil {
		return err
	}
	s.SetupConfig.Notifications.Telegram.Enabled = enableTelegram

	if enableTelegram {
		tokenPrompt := &survey.Input{
			Message: "Telegram bot token:",
			Default: s.SetupConfig.Notifications.Telegram.BotToken,
		}
		if err := survey.AskOne(tokenPrompt, &s.SetupConfig.Notifications.Telegram.BotToken); err != nil {
			return err
		}

		chatPrompt := &survey.Input{
			Message: "Telegram chat ID:",
			Default: s.SetupConfig.Notifications.Telegram.ChatID,
		}
		if err := survey.AskOne(chatPrompt, &s.SetupConfig.Notifications.Telegram.ChatID); err != nil {
			return err
		}
	}

	// Slack
	var enableSlack bool
	slackPrompt := &survey.Confirm{
		Message: "Enable Slack notifications?",
		Default: s.SetupConfig.Notifications.Slack.Enabled,
	}
	if err := survey.AskOne(slackPrompt, &enableSlack); err != nil {
		return err
	}
	s.SetupConfig.Notifications.Slack.Enabled = enableSlack

	if enableSlack {
		webhookPrompt := &survey.Input{
			Message: "Slack webhook URL:",
			Default: s.SetupConfig.Notifications.Slack.WebhookURL,
		}
		if err := survey.AskOne(webhookPrompt, &s.SetupConfig.Notifications.Slack.WebhookURL); err != nil {
			return err
		}
	}

	return nil
}

// getPublicRestTransports returns names of REST transports with public_service=true
func (s *Setup) getPublicRestTransports() []string {
	var result []string
	for _, rest := range s.ProjectConfig.RestList {
		if rest.PublicService {
			result = append(result, rest.Name)
		}
	}
	return result
}
