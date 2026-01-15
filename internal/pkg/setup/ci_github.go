package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	registryTypeGitHub       = "github"
	registryTypeDigitalOcean = "digitalocean"
	registryTypeAWS          = "aws"
	registryTypeSelfHosted   = "selfhosted"

	placeholderSSHKey  = "<your-deploy-ssh-private-key>"
	defaultDeployUser  = "deploy"
	cliArgBody         = "--body"
	formatKeyValue     = "  %s: %s\n"
	formatFailedSetVar = "Failed to set variable %s: %v"
	codeBlockDelimiter = "```"
	codeBlockBash      = "```bash"
	shellEscapeQuote   = "'\\''"
)

// setupGitHub configures GitHub Actions secrets and variables
func (s *Setup) setupGitHub() error {
	// Check if gh CLI is available
	ghAvailable := isCommandAvailable("gh")

	if ghAvailable {
		PrintInfo("GitHub CLI (gh) detected. Will use it to configure secrets and variables.")
	} else {
		PrintWarning("GitHub CLI (gh) not found. Will provide manual instructions.")
	}

	// Generate secrets
	secrets := s.generateGitHubSecrets()
	variables := s.generateGitHubVariables()

	if ghAvailable {
		return s.setupGitHubWithCLI(secrets, variables)
	}

	return s.printGitHubManualInstructions(secrets, variables)
}

// generateGitHubSecrets generates the list of secrets needed
func (s *Setup) generateGitHubSecrets() map[string]string {
	secrets := make(map[string]string)

	// SSH secrets
	secrets["SSH_PRIVATE_KEY"] = placeholderSSHKey
	secrets["SSH_USER"] = defaultDeployUser

	// Registry secrets based on type
	secrets["REGISTRY_LOGIN_SERVER"] = s.SetupConfig.Registry.Server

	switch s.SetupConfig.Registry.Type {
	case registryTypeGitHub:
		secrets["GHCR_USER"] = "<your-github-username>"
		secrets["GHCR_TOKEN"] = "<your-github-token>"
	case registryTypeDigitalOcean:
		secrets["REGISTRY_PASSWORD"] = "<your-digitalocean-api-token>"
	case registryTypeAWS:
		secrets["AWS_ACCESS_KEY_ID"] = "<your-aws-access-key>"
		secrets["AWS_SECRET_ACCESS_KEY"] = "<your-aws-secret-key>"
		secrets["AWS_REGION"] = "<your-aws-region>"
	case registryTypeSelfHosted:
		secrets["REGISTRY_URL"] = s.SetupConfig.Registry.Server
		secrets["REGISTRY_USERNAME"] = "<your-registry-username>"
		secrets["REGISTRY_PASSWORD"] = "<your-registry-password>"
	}

	// GitHub PAT for private repos
	secrets["GH_PAT"] = "<your-github-personal-access-token>"

	// Notifications
	if s.SetupConfig.Notifications.Telegram.Enabled {
		secrets["TELEGRAM_BOT_TOKEN"] = s.SetupConfig.Notifications.Telegram.BotToken
		secrets["TELEGRAM_CHAT_ID"] = s.SetupConfig.Notifications.Telegram.ChatID
	}

	if s.SetupConfig.Notifications.Slack.Enabled {
		secrets["ACTION_MONITORING_SLACK"] = s.SetupConfig.Notifications.Slack.WebhookURL
	}

	// OnlineConf secrets per branch
	for _, env := range s.SetupConfig.Environments {
		branch := strings.ToUpper(env.Branch)
		// JSON array format for multi-instance support
		secrets[fmt.Sprintf("%s_OC_USER", branch)] = fmt.Sprintf(`["%s"]`, env.OnlineConf.User)
		secrets[fmt.Sprintf("%s_OC_PASSWORD", branch)] = `["<onlineconf-password>"]`
	}

	return secrets
}

// generateGitHubVariables generates the list of variables needed
func (s *Setup) generateGitHubVariables() map[string]string {
	variables := make(map[string]string)

	// Registry container
	variables["REGISTRY_CONTAINER"] = s.SetupConfig.Registry.Container

	// Per-branch variables
	for _, env := range s.SetupConfig.Environments {
		branch := strings.ToUpper(env.Branch)

		variables[fmt.Sprintf("%s_ENABLED", branch)] = "ENABLED"
		variables[fmt.Sprintf("%s_ENV_TYPE", branch)] = `[""]` // Empty for single instance

		// SSH hosts as JSON array: [["public_ip", "internal_ip"]]
		sshHost, _ := json.Marshal([][]string{{env.Server.Host, env.Server.Host}})
		variables[fmt.Sprintf("%s_SSH_HOST", branch)] = string(sshHost)

		// OnlineConf
		variables[fmt.Sprintf("%s_OC_HOST", branch)] = env.OnlineConf.Host
		variables[fmt.Sprintf("%s_OC_PORT", branch)] = fmt.Sprintf("%d", env.OnlineConf.Port)

		// Internal subnet
		variables[fmt.Sprintf("%s_INTERNAL_SUBNET", branch)] = env.InternalSubnet

		// Port prefixes for services
		for _, svc := range env.Services {
			svcName := strings.ToUpper(strings.ReplaceAll(svc.Name, "-", "_"))

			// Traefik port
			portJSON, _ := json.Marshal([]string{fmt.Sprintf("%d", svc.PortPrefix)})
			variables[fmt.Sprintf("%s_PORT_PREFIX_%s_TRAEFIK", branch, svcName)] = string(portJSON)

			// If service has domain, add domain variable
			if svc.Domain != "" {
				domainJSON, _ := json.Marshal([]string{svc.Domain})
				// Try to find the REST transport name for this service
				for _, rest := range s.ProjectConfig.RestList {
					if rest.PublicService {
						restName := strings.ToUpper(rest.Name)
						variables[fmt.Sprintf("%s_DOMAIN_%s", branch, restName)] = string(domainJSON)
						variables[fmt.Sprintf("%s_PORT_PREFIX_%s_%s", branch, svcName, restName)] = string(portJSON)
					}
				}
			}
		}
	}

	return variables
}

// setupGitHubWithCLI uses gh CLI to configure secrets and variables
func (s *Setup) setupGitHubWithCLI(secrets, variables map[string]string) error {
	repo := s.SetupConfig.CI.Repo

	fmt.Println("\nConfiguring GitHub secrets...")

	reader := bufio.NewReader(os.Stdin)

	for name, value := range secrets {
		if strings.Contains(value, "<") {
			// This is a placeholder - ask for actual value
			fmt.Printf("\nSecret: %s\n", name)
			if value != placeholderSSHKey {
				var actualValue string
				fmt.Printf("Enter value (current placeholder: %s): ", value)

				line, err := reader.ReadString('\n')
				if err != nil {
					PrintWarning(fmt.Sprintf("Skipping %s - failed to read input", name))
					continue
				}

				actualValue = strings.TrimSpace(line)

				if actualValue == "" {
					PrintWarning(fmt.Sprintf("Skipping %s - no value provided", name))
					continue
				}

				value = actualValue
			} else {
				PrintWarning(fmt.Sprintf("Skipping %s - SSH key should be set manually", name))
				continue
			}
		}

		if s.DryRun {
			fmt.Printf("  [DRY-RUN] Would set secret: %s\n", name)
			continue
		}

		cmd := exec.Command("gh", "secret", "set", name, "-R", repo, cliArgBody, value)
		if err := cmd.Run(); err != nil {
			PrintError(fmt.Sprintf("Failed to set secret %s: %v", name, err))
		} else {
			PrintSuccess(fmt.Sprintf("Set secret: %s", name))
		}
	}

	fmt.Println("\nConfiguring GitHub variables...")

	for name, value := range variables {
		if s.DryRun {
			fmt.Printf("  [DRY-RUN] Would set variable: %s = %s\n", name, value)
			continue
		}

		cmd := exec.Command("gh", "variable", "set", name, "-R", repo, cliArgBody, value)
		if err := cmd.Run(); err != nil {
			PrintError(fmt.Sprintf(formatFailedSetVar, name, err))
		} else {
			PrintSuccess(fmt.Sprintf("Set variable: %s", name))
		}
	}

	return nil
}

// printGitHubManualInstructions prints manual setup instructions
func (s *Setup) printGitHubManualInstructions(secrets, variables map[string]string) error {
	repo := s.SetupConfig.CI.Repo

	fmt.Printf("\n=== GitHub Manual Setup Instructions ===\n")
	fmt.Printf("\nGo to: https://github.com/%s/settings/secrets/actions\n", repo)

	fmt.Println("\n--- Secrets to create ---")
	for name, value := range secrets {
		fmt.Printf(formatKeyValue, name, value)
	}

	fmt.Printf("\nGo to: https://github.com/%s/settings/variables/actions\n", repo)

	fmt.Println("\n--- Variables to create ---")
	for name, value := range variables {
		fmt.Printf(formatKeyValue, name, value)
	}

	// Also print gh commands for copy-paste
	fmt.Println("\n--- Or use these gh commands ---")
	fmt.Println(codeBlockBash)
	for name, value := range secrets {
		// Escape value for shell
		escapedValue := strings.ReplaceAll(value, "'", shellEscapeQuote)
		fmt.Printf("gh secret set %s -R %s --body '%s'\n", name, repo, escapedValue)
	}
	for name, value := range variables {
		escapedValue := strings.ReplaceAll(value, "'", shellEscapeQuote)
		fmt.Printf("gh variable set %s -R %s --body '%s'\n", name, repo, escapedValue)
	}

	fmt.Println(codeBlockDelimiter)

	return nil
}

// isCommandAvailable checks if a command is available in PATH
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
