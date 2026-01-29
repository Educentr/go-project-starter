package setup

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

const (
	// Default values
	defaultSSHPort        = 22
	defaultPortPrefix     = 80
	defaultOnlineConfPort = 443
	defaultInternalSubnet = "10.0.0.0/8"
	warnNoServersConf     = "No servers configured"
	warnNoAppsConf        = "No applications configured"
	splitCountSSHURL      = 2
)

// runWizard runs the interactive setup wizard
func (s *Setup) runWizard() error {
	// Step 1: Admin email
	if err := s.askAdminEmail(); err != nil {
		return err
	}

	// Step 2: CI/CD provider (auto-detect from project.yaml)
	if err := s.askCIProvider(); err != nil {
		return err
	}

	// Step 3: Repository (auto from git.repo)
	if err := s.askRepository(); err != nil {
		return err
	}

	// Step 4: Registry configuration (auto from project.yaml)
	if err := s.askRegistry(); err != nil {
		return err
	}

	// Step 5: Servers
	if err := s.askServers(); err != nil {
		return err
	}

	// Step 6: Applications (auto-populate from project.yaml)
	if err := s.askApplications(); err != nil {
		return err
	}

	// Step 7: Environments and deployments
	if err := s.askEnvironments(); err != nil {
		return err
	}

	// Step 8: Notifications
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
	// Auto-detect from git.repo
	gitRepo := s.ProjectConfig.Git.Repo
	if strings.Contains(gitRepo, "github.com") {
		s.SetupConfig.CI.Provider = ciProviderGitHub
		PrintInfo("Detected CI provider: " + ciProviderGitHub)

		return nil
	} else if strings.Contains(gitRepo, ciProviderGitLab) {
		s.SetupConfig.CI.Provider = ciProviderGitLab
		PrintInfo("Detected CI provider: " + ciProviderGitLab)

		return nil
	}

	options := []string{ciProviderGitHub, ciProviderGitLab}
	prompt := &survey.Select{
		Message: "CI/CD provider:",
		Options: options,
		Default: s.SetupConfig.CI.Provider,
	}

	return survey.AskOne(prompt, &s.SetupConfig.CI.Provider)
}

func (s *Setup) askRepository() error {
	// Try to extract owner/repo from project.yaml git.repo
	defaultRepo := s.SetupConfig.CI.Repo
	if defaultRepo == "" {
		defaultRepo = extractRepoFromGitURL(s.ProjectConfig.Git.Repo)
	}

	// If we have a valid repo from project.yaml, use it
	if defaultRepo != "" && s.SetupConfig.CI.Repo == "" {
		s.SetupConfig.CI.Repo = defaultRepo
		PrintInfo(fmt.Sprintf("Using repository from project.yaml: %s", defaultRepo))

		return nil
	}

	message := "Repository (owner/repo):"
	if s.SetupConfig.CI.Provider == "gitlab" {
		message = "Repository (group/project or full path):"
	}

	prompt := &survey.Input{
		Message: message,
		Default: defaultRepo,
	}
	return survey.AskOne(prompt, &s.SetupConfig.CI.Repo, survey.WithValidator(survey.Required))
}

// extractRepoFromGitURL extracts owner/repo from git URL
func extractRepoFromGitURL(gitURL string) string {
	if gitURL == "" {
		return ""
	}

	gitURL = strings.TrimSuffix(gitURL, ".git")

	if strings.HasPrefix(gitURL, "git@") {
		parts := strings.SplitN(gitURL, ":", splitCountSSHURL)
		if len(parts) == splitCountSSHURL {
			return parts[1]
		}
	}

	if strings.HasPrefix(gitURL, "https://") || strings.HasPrefix(gitURL, "http://") {
		parts := strings.SplitN(gitURL, "//", splitCountSSHURL)
		if len(parts) == splitCountSSHURL {
			pathParts := strings.SplitN(parts[1], "/", splitCountSSHURL)
			if len(pathParts) == splitCountSSHURL {
				return pathParts[1]
			}
		}
	}

	return ""
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

	// Set default server based on type
	switch s.SetupConfig.Registry.Type {
	case "github":
		s.SetupConfig.Registry.Server = "ghcr.io"
	case "digitalocean":
		s.SetupConfig.Registry.Server = "registry.digitalocean.com"
	}

	// Container path - default to CI repo
	if s.SetupConfig.Registry.Container == "" && s.SetupConfig.CI.Repo != "" {
		s.SetupConfig.Registry.Container = s.SetupConfig.CI.Repo
		PrintInfo(fmt.Sprintf("Using container path: %s", s.SetupConfig.Registry.Container))
	}

	return nil
}

func (s *Setup) askServers() error {
	fmt.Println("\n--- Servers ---")

	count, err := s.askServerCount()
	if err != nil {
		return err
	}

	// Ensure we have enough server slots
	for len(s.SetupConfig.Servers) < count {
		s.SetupConfig.Servers = append(s.SetupConfig.Servers, ServerConfig{
			SSHPort:    defaultSSHPort,
			SSHUser:    "root",
			DeployUser: "deploy",
		})
	}

	s.SetupConfig.Servers = s.SetupConfig.Servers[:count]

	// Configure each server
	for i := range count {
		if err := s.configureServer(i, count); err != nil {
			return err
		}
	}

	return nil
}

func (s *Setup) askServerCount() (int, error) {
	var serverCount string
	if len(s.SetupConfig.Servers) > 0 {
		serverCount = strconv.Itoa(len(s.SetupConfig.Servers))
	} else {
		serverCount = "1"
	}

	prompt := &survey.Select{
		Message: "How many deployment servers?",
		Options: []string{"1", "2", "3", "4", "5"},
		Default: serverCount,
	}

	if err := survey.AskOne(prompt, &serverCount); err != nil {
		return 0, err
	}

	count, _ := strconv.Atoi(serverCount) //nolint:errcheck // serverCount comes from fixed Select options
	if count == 0 {
		count = 1
	}

	return count, nil
}

func (s *Setup) configureServer(i, count int) error {
	fmt.Printf("\n--- Server %d ---\n", i+1)
	srv := &s.SetupConfig.Servers[i]

	// Name
	defaultName := srv.Name
	if defaultName == "" {
		if count == 1 {
			defaultName = "production"
		} else {
			defaultName = fmt.Sprintf("server-%d", i+1)
		}
	}

	namePrompt := &survey.Input{
		Message: "Server name (logical identifier):",
		Default: defaultName,
	}

	if err := survey.AskOne(namePrompt, &srv.Name, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// Host
	hostPrompt := &survey.Input{
		Message: "Server host (IP or hostname):",
		Default: srv.Host,
	}

	if err := survey.AskOne(hostPrompt, &srv.Host, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	// SSH port
	if srv.SSHPort == 0 {
		srv.SSHPort = defaultSSHPort
	}

	var portStr string

	portPrompt := &survey.Input{
		Message: "SSH port:",
		Default: strconv.Itoa(srv.SSHPort),
	}

	if err := survey.AskOne(portPrompt, &portStr); err != nil {
		return err
	}

	if port, err := strconv.Atoi(portStr); err == nil {
		srv.SSHPort = port
	}

	// SSH user
	if srv.SSHUser == "" {
		srv.SSHUser = "root"
	}

	userPrompt := &survey.Input{
		Message: "SSH user (for initial setup):",
		Default: srv.SSHUser,
	}

	if err := survey.AskOne(userPrompt, &srv.SSHUser); err != nil {
		return err
	}

	// Deploy user
	if srv.DeployUser == "" {
		srv.DeployUser = "deploy"
	}

	deployPrompt := &survey.Input{
		Message: "Deploy user (will be created):",
		Default: srv.DeployUser,
	}

	return survey.AskOne(deployPrompt, &srv.DeployUser)
}

func (s *Setup) askApplications() error {
	fmt.Println("\n--- Applications ---")

	// Auto-populate from project.yaml
	projectApps := s.GetApplications()
	if len(projectApps) == 0 {
		PrintWarning("No applications found in project.yaml")
		return nil
	}

	// Build applications list with auto-detected properties
	if len(s.SetupConfig.Applications) == 0 {
		for _, appName := range projectApps {
			app := ApplicationConfig{
				Name:       appName,
				Singleton:  s.isAppSingleton(appName),
				PortPrefix: defaultPortPrefix,
			}
			s.SetupConfig.Applications = append(s.SetupConfig.Applications, app)
		}
	}

	// Display applications info
	fmt.Println("\nDetected applications from project.yaml:")

	for i, app := range s.SetupConfig.Applications {
		singletonStr := ""

		if app.Singleton {
			singletonStr = " [SINGLETON]"
		}

		fmt.Printf("  %d. %s%s\n", i+1, app.Name, singletonStr)
	}

	// Ask if need to configure port prefixes
	var configureApps bool

	configPrompt := &survey.Confirm{
		Message: "Configure port prefixes for applications?",
		Default: false,
	}

	if err := survey.AskOne(configPrompt, &configureApps); err != nil {
		return err
	}

	if configureApps {
		for i := range s.SetupConfig.Applications {
			app := &s.SetupConfig.Applications[i]

			var portStr string

			portPrompt := &survey.Input{
				Message: fmt.Sprintf("Port prefix for %s:", app.Name),
				Default: strconv.Itoa(app.PortPrefix),
			}

			if err := survey.AskOne(portPrompt, &portStr); err != nil {
				return err
			}

			port, err := strconv.Atoi(portStr)
			if err == nil {
				app.PortPrefix = port
			}
		}
	}

	return nil
}

// isAppSingleton checks if application should be singleton based on its configuration
func (s *Setup) isAppSingleton(appName string) bool {
	for _, app := range s.ProjectConfig.Applications {
		if app.Name != appName {
			continue
		}

		// Check workers for singleton patterns
		for _, workerName := range app.WorkerList {
			if worker, ok := s.ProjectConfig.WorkerMap[workerName]; ok {
				// Telegram bot with poller is singleton
				if worker.GeneratorTemplate == "telegram" {
					// Check driver params for poller
					for _, driver := range app.DriverList {
						for _, param := range driver.Params {
							if strings.Contains(param, "UpdatePoller") {
								return true
							}
						}
					}
				}

				// Daemon workers are typically singleton
				if worker.GeneratorTemplate == "daemon" {
					return true
				}
			}
		}
	}

	return false
}

func (s *Setup) askEnvironments() error {
	fmt.Println("\n--- Environments ---")

	// Ask how many environments
	var envCount string
	if len(s.SetupConfig.Environments) > 0 {
		envCount = strconv.Itoa(len(s.SetupConfig.Environments))
	} else {
		envCount = "1"
	}

	prompt := &survey.Select{
		Message: "How many environments? (production, staging, etc.)",
		Options: []string{"1", "2"},
		Default: envCount,
	}

	if err := survey.AskOne(prompt, &envCount); err != nil {
		return err
	}

	count, _ := strconv.Atoi(envCount) //nolint:errcheck // envCount comes from fixed Select options
	if count == 0 {
		count = 1
	}

	// Ensure we have enough environment slots
	for len(s.SetupConfig.Environments) < count {
		s.SetupConfig.Environments = append(s.SetupConfig.Environments, EnvironmentConfig{
			InternalSubnet: defaultInternalSubnet,
			OnlineConf: OnlineConfConfig{
				Port: defaultOnlineConfPort,
			},
		})
	}

	s.SetupConfig.Environments = s.SetupConfig.Environments[:count]

	// Default names and branches
	defaultNames := []string{"production", "staging"}
	defaultBranches := []string{"main", "staging"}

	for i := range count {
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

		// OnlineConf
		if err := s.askOnlineConf(env); err != nil {
			return err
		}

		// Internal subnet
		if err := s.askInternalSubnet(env); err != nil {
			return err
		}

		// Deployments - which apps on which servers
		if err := s.askDeployments(env); err != nil {
			return err
		}
	}

	return nil
}

func (s *Setup) askOnlineConf(env *EnvironmentConfig) error {
	// Ask if OnlineConf is used
	var useOnlineConf bool

	// Default to true if host is already set
	defaultUseOnlineConf := env.OnlineConf.Host != ""

	usePrompt := &survey.Confirm{
		Message: "Use OnlineConf for configuration management?",
		Default: defaultUseOnlineConf,
		Help:    "If disabled, the service will use environment variables for configuration",
	}

	if err := survey.AskOne(usePrompt, &useOnlineConf); err != nil {
		return err
	}

	env.OnlineConf.Enabled = useOnlineConf

	if !useOnlineConf {
		PrintInfo("OnlineConf disabled - service will use environment variables")

		return nil
	}

	fmt.Println("\n  OnlineConf configuration:")

	hostPrompt := &survey.Input{
		Message: "  OnlineConf host:",
		Default: env.OnlineConf.Host,
	}

	if err := survey.AskOne(hostPrompt, &env.OnlineConf.Host, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	if env.OnlineConf.Port == 0 {
		env.OnlineConf.Port = defaultOnlineConfPort
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

	return nil
}

func (s *Setup) askInternalSubnet(env *EnvironmentConfig) error {
	var hasInternalNetwork bool

	// Default to true if subnet is already set
	defaultHasInternal := env.InternalSubnet != ""

	networkPrompt := &survey.Confirm{
		Message: "Does your infrastructure have an internal network?",
		Default: defaultHasInternal,
		Help:    "Internal network is used to restrict access to diagnostic endpoints (metrics, health checks)",
	}

	if err := survey.AskOne(networkPrompt, &hasInternalNetwork); err != nil {
		return err
	}

	if !hasInternalNetwork {
		env.InternalSubnet = ""

		PrintWarning("WARNING: Without internal network, diagnostic endpoints (sys) will be exposed publicly!")
		PrintWarning("Consider using firewall rules to restrict access to port 8085 (metrics/health)")

		return nil
	}

	if env.InternalSubnet == "" {
		env.InternalSubnet = defaultInternalSubnet
	}

	subnetPrompt := &survey.Input{
		Message: "Internal subnet (CIDR notation):",
		Default: env.InternalSubnet,
		Help:    "Example: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16",
	}

	return survey.AskOne(subnetPrompt, &env.InternalSubnet)
}

func (s *Setup) askDeployments(env *EnvironmentConfig) error {
	fmt.Println("\n  Deployment mapping (which apps run on which servers):")

	if len(s.SetupConfig.Servers) == 0 {
		PrintWarning(warnNoServersConf)

		return nil
	}

	if len(s.SetupConfig.Applications) == 0 {
		PrintWarning(warnNoAppsConf)

		return nil
	}

	// Get server names
	serverNames := make([]string, len(s.SetupConfig.Servers))
	for i, srv := range s.SetupConfig.Servers {
		serverNames[i] = srv.Name
	}

	// Get app names
	appNames := make([]string, len(s.SetupConfig.Applications))
	for i, app := range s.SetupConfig.Applications {
		appNames[i] = app.Name
	}

	// Simple case: 1 server - all apps go there
	if len(s.SetupConfig.Servers) == 1 {
		env.Deployments = []DeploymentConfig{
			{
				Server: serverNames[0],
				Apps:   appNames,
			},
		}

		PrintInfo(fmt.Sprintf("All applications will be deployed to %s", serverNames[0]))

		return nil
	}

	// Multiple servers - ask for each server which apps to deploy
	env.Deployments = nil

	for _, serverName := range serverNames {
		var selectedApps []string

		appsPrompt := &survey.MultiSelect{
			Message: fmt.Sprintf("  Select applications for server '%s':", serverName),
			Options: appNames,
		}
		if err := survey.AskOne(appsPrompt, &selectedApps); err != nil {
			return err
		}

		if len(selectedApps) > 0 {
			env.Deployments = append(env.Deployments, DeploymentConfig{
				Server: serverName,
				Apps:   selectedApps,
			})
		}
	}

	// Validate singleton constraints
	if err := s.SetupConfig.ValidateDeployments(); err != nil {
		PrintError(err.Error())
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
