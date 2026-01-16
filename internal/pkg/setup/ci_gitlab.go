package setup

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitLabVariable represents a GitLab CI/CD variable
type GitLabVariable struct {
	Key       string
	Value     string
	Protected bool
	Masked    bool
	Scope     string // environment scope: *, production, staging
}

const (
	cliGlab         = "glab"
	flagProtected   = "Protected"
	flagMasked      = "Masked"
	formatFlagStr   = " [%s]"
	formatGitLabVar = "  %s: %s%s\n"
)

// setupGitLab configures GitLab CI/CD variables
func (s *Setup) setupGitLab() error {
	// Check if glab CLI is available
	glabAvailable := isCommandAvailable(cliGlab)

	if glabAvailable {
		PrintInfo("GitLab CLI (glab) detected. Will use it to configure variables.")
	} else {
		PrintWarning("GitLab CLI (glab) not found. Will provide manual instructions.")
	}

	// Generate variables (GitLab uses variables for both secrets and vars)
	variables := s.generateGitLabVariables()

	if glabAvailable {
		return s.setupGitLabWithCLI(variables)
	}

	return s.printGitLabManualInstructions(variables)
}

// generateGitLabVariables generates the list of variables needed
func (s *Setup) generateGitLabVariables() []GitLabVariable {
	var variables []GitLabVariable

	// SSH secrets (protected, masked)
	variables = append(variables, GitLabVariable{
		Key:       "SSH_PRIVATE_KEY",
		Value:     "<your-deploy-ssh-private-key>",
		Protected: true,
		Masked:    false, // SSH keys are too long to mask
		Scope:     "*",
	})
	variables = append(variables, GitLabVariable{
		Key:       "SSH_USER",
		Value:     "deploy",
		Protected: true,
		Masked:    false,
		Scope:     "*",
	})

	// Registry credentials
	variables = append(variables, GitLabVariable{
		Key:       "REGISTRY_LOGIN_SERVER",
		Value:     s.SetupConfig.Registry.Server,
		Protected: false,
		Masked:    false,
		Scope:     "*",
	})

	switch s.SetupConfig.Registry.Type {
	case registryTypeGitHub:
		variables = append(variables, GitLabVariable{
			Key:       "GHCR_USER",
			Value:     "<your-github-username>",
			Protected: true,
			Masked:    false,
			Scope:     "*",
		})
		variables = append(variables, GitLabVariable{
			Key:       "GHCR_TOKEN",
			Value:     "<your-github-token>",
			Protected: true,
			Masked:    true,
			Scope:     "*",
		})
	case registryTypeDigitalOcean:
		variables = append(variables, GitLabVariable{
			Key:       "REGISTRY_PASSWORD",
			Value:     "<your-digitalocean-api-token>",
			Protected: true,
			Masked:    true,
			Scope:     "*",
		})
	case registryTypeSelfHosted:
		variables = append(variables, GitLabVariable{
			Key:       "REGISTRY_URL",
			Value:     s.SetupConfig.Registry.Server,
			Protected: false,
			Masked:    false,
			Scope:     "*",
		})
		variables = append(variables, GitLabVariable{
			Key:       "REGISTRY_USERNAME",
			Value:     "<your-registry-username>",
			Protected: true,
			Masked:    false,
			Scope:     "*",
		})
		variables = append(variables, GitLabVariable{
			Key:       "REGISTRY_PASSWORD",
			Value:     "<your-registry-password>",
			Protected: true,
			Masked:    true,
			Scope:     "*",
		})
	}

	// Registry container
	variables = append(variables, GitLabVariable{
		Key:       "REGISTRY_CONTAINER",
		Value:     s.SetupConfig.Registry.Container,
		Protected: false,
		Masked:    false,
		Scope:     "*",
	})

	// Notifications
	if s.SetupConfig.Notifications.Telegram.Enabled {
		variables = append(variables, GitLabVariable{
			Key:       "TELEGRAM_BOT_TOKEN",
			Value:     s.SetupConfig.Notifications.Telegram.BotToken,
			Protected: true,
			Masked:    true,
			Scope:     "*",
		})
		variables = append(variables, GitLabVariable{
			Key:       "TELEGRAM_CHAT_ID",
			Value:     s.SetupConfig.Notifications.Telegram.ChatID,
			Protected: true,
			Masked:    false,
			Scope:     "*",
		})
	}

	if s.SetupConfig.Notifications.Slack.Enabled {
		variables = append(variables, GitLabVariable{
			Key:       "SLACK_WEBHOOK_URL",
			Value:     s.SetupConfig.Notifications.Slack.WebhookURL,
			Protected: true,
			Masked:    true,
			Scope:     "*",
		})
	}

	// Per-environment variables
	for _, env := range s.SetupConfig.Environments {
		scope := env.Branch // Use branch as environment scope

		// OnlineConf credentials (only if enabled)
		if env.OnlineConf.Enabled && env.OnlineConf.Host != "" {
			variables = append(variables, GitLabVariable{
				Key:       "OC_USER",
				Value:     env.OnlineConf.User,
				Protected: true,
				Masked:    false,
				Scope:     scope,
			})
			variables = append(variables, GitLabVariable{
				Key:       "OC_PASSWORD",
				Value:     "<onlineconf-password>",
				Protected: true,
				Masked:    true,
				Scope:     scope,
			})
			variables = append(variables, GitLabVariable{
				Key:       "OC_HOST",
				Value:     env.OnlineConf.Host,
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})
			variables = append(variables, GitLabVariable{
				Key:       "OC_PORT",
				Value:     fmt.Sprintf("%d", env.OnlineConf.Port),
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})
		}

		// SSH hosts from deployments (comma-separated for multiple servers)
		var sshHosts []string

		for _, deployment := range env.Deployments {
			server := s.SetupConfig.GetServerByName(deployment.Server)
			if server != nil {
				sshHosts = append(sshHosts, server.Host)
			}
		}

		if len(sshHosts) > 0 {
			variables = append(variables, GitLabVariable{
				Key:       "SSH_HOST",
				Value:     strings.Join(sshHosts, ","),
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})
		}

		// Internal subnet
		if env.InternalSubnet != "" {
			variables = append(variables, GitLabVariable{
				Key:       "INTERNAL_SUBNET",
				Value:     env.InternalSubnet,
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})
		}

		// Port prefixes for applications
		for _, app := range s.SetupConfig.Applications {
			appName := strings.ToUpper(strings.ReplaceAll(app.Name, "-", "_"))
			variables = append(variables, GitLabVariable{
				Key:       fmt.Sprintf("PORT_PREFIX_%s", appName),
				Value:     fmt.Sprintf("%d", app.PortPrefix),
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})

			if app.Domain != "" {
				variables = append(variables, GitLabVariable{
					Key:       fmt.Sprintf("DOMAIN_%s", appName),
					Value:     app.Domain,
					Protected: false,
					Masked:    false,
					Scope:     scope,
				})
			}
		}

		// Apps per deployment
		for i, deployment := range env.Deployments {
			variables = append(variables, GitLabVariable{
				Key:       fmt.Sprintf("APPS_%d", i),
				Value:     strings.Join(deployment.Apps, ","),
				Protected: false,
				Masked:    false,
				Scope:     scope,
			})
		}
	}

	return variables
}

// setupGitLabWithCLI uses glab CLI to configure variables
func (s *Setup) setupGitLabWithCLI(variables []GitLabVariable) error {
	repo := s.SetupConfig.CI.Repo

	fmt.Println("\nConfiguring GitLab CI/CD variables...")

	for _, v := range variables {
		if strings.Contains(v.Value, "<") {
			// Placeholder - skip or ask
			PrintWarning(fmt.Sprintf("Skipping %s - placeholder value, set manually", v.Key))
			continue
		}

		if s.DryRun {
			fmt.Printf("  [DRY-RUN] Would set variable: %s (scope: %s)\n", v.Key, v.Scope)
			continue
		}

		// Build glab command
		args := []string{"variable", "set", v.Key, "--value", v.Value, "-R", repo}

		if v.Protected {
			args = append(args, "--protected")
		}
		if v.Masked {
			args = append(args, "--masked")
		}
		if v.Scope != "*" {
			args = append(args, "--scope", v.Scope)
		}

		cmd := exec.Command("glab", args...)
		if err := cmd.Run(); err != nil {
			PrintError(fmt.Sprintf("Failed to set variable %s: %v", v.Key, err))
		} else {
			PrintSuccess(fmt.Sprintf("Set variable: %s (scope: %s)", v.Key, v.Scope))
		}
	}

	return nil
}

// printGitLabManualInstructions prints manual setup instructions
func (s *Setup) printGitLabManualInstructions(variables []GitLabVariable) error {
	repo := s.SetupConfig.CI.Repo

	// Try to construct GitLab URL
	gitlabURL := "https://gitlab.com"
	if strings.Contains(repo, ".") {
		// Might be self-hosted GitLab
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) > 0 {
			gitlabURL = fmt.Sprintf("https://%s", parts[0])
		}
	}

	fmt.Printf("\n=== GitLab Manual Setup Instructions ===\n")
	fmt.Printf("\nGo to: %s/%s/-/settings/ci_cd\n", gitlabURL, repo)
	fmt.Println("Expand 'Variables' section and add the following:")

	// Group by scope
	scopes := make(map[string][]GitLabVariable)
	for _, v := range variables {
		scopes[v.Scope] = append(scopes[v.Scope], v)
	}

	// Print global variables first
	if globals, ok := scopes["*"]; ok {
		fmt.Println("--- Global Variables (All Environments) ---")
		for _, v := range globals {
			fmt.Printf(formatGitLabVar, v.Key, v.Value, formatVarFlags(v))
		}

		delete(scopes, "*")
	}

	// Print environment-specific variables
	for scope, vars := range scopes {
		fmt.Printf("\n--- Environment: %s ---\n", scope)
		for _, v := range vars {
			fmt.Printf(formatGitLabVar, v.Key, v.Value, formatVarFlags(v))
		}
	}

	return nil
}

// formatVarFlags returns formatted flags string for a GitLab variable
func formatVarFlags(v GitLabVariable) string {
	var flags []string

	if v.Protected {
		flags = append(flags, flagProtected)
	}

	if v.Masked {
		flags = append(flags, flagMasked)
	}

	if len(flags) > 0 {
		return fmt.Sprintf(formatFlagStr, strings.Join(flags, ", "))
	}

	return ""
}
