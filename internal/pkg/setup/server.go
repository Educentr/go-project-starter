package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// SSHCheckResult represents the result of SSH connectivity check
type SSHCheckResult int

const (
	SSHCheckOK SSHCheckResult = iota
	SSHCheckHostKeyChanged
	SSHCheckAuthFailed
	SSHCheckConnectionFailed
	SSHCheckUnknownError
)

const (
	sshCommand             = "ssh"
	sshKeygenCommand       = "ssh-keygen"
	sshDir                 = ".ssh"
	sshOptionStrictHostKey = "StrictHostKeyChecking=no"
	formatUserAtHost       = "%s@%s"
	envHome                = "HOME"
	portMultiplier         = 100
	portOffset             = 80
	portOffsetSys          = 85
	defaultSSHPortNum      = 22

	pubKeyExtension       = ".pub"
	optionCustomPath      = "Enter custom path..."
	sshTestCommand        = "echo ok"
	filePermPrivate       = 0600
	filePermPublic        = 0644
	formatSSHKeyUploadCmd = "  gh secret set SSH_PRIVATE_KEY -R %s < %s\n"
)

// ErrSSHSetupCanceled is returned when SSH setup is canceled by user
var ErrSSHSetupCanceled = fmt.Errorf("SSH setup canceled")

// setupAllServers configures all servers
func (s *Setup) setupAllServers() error {
	if len(s.SetupConfig.Servers) == 0 {
		PrintWarning("No servers configured")
		return nil
	}

	for _, srv := range s.SetupConfig.Servers {
		fmt.Printf("\n=== Setting up server: %s (%s) ===\n", srv.Name, srv.Host)

		if err := s.setupServer(srv); err != nil {
			return fmt.Errorf("failed to setup server %s: %w", srv.Name, err)
		}
	}

	return nil
}

// setupServer configures a single server
func (s *Setup) setupServer(srv ServerConfig) error {
	// Check SSH connectivity with detailed error handling
	sshResult, sshAvailable, err := s.checkSSHAccessDetailed(srv)
	if err != nil {
		return err
	}

	if sshAvailable {
		PrintSuccess(fmt.Sprintf("SSH access to %s verified", srv.Host))
	} else {
		switch sshResult {
		case SSHCheckHostKeyChanged:
			// Already handled in checkSSHAccessDetailed
			return fmt.Errorf("%w for server %s", ErrSSHSetupCanceled, srv.Name)
		case SSHCheckAuthFailed:
			PrintWarning(fmt.Sprintf("SSH authentication failed for %s. Will provide manual instructions.", srv.Host))
		case SSHCheckConnectionFailed:
			PrintWarning(fmt.Sprintf("Cannot connect to %s (connection refused/timeout). Will provide manual instructions.", srv.Host))
		default:
			PrintWarning(fmt.Sprintf("Cannot connect to %s via SSH. Will provide manual instructions.", srv.Host))
		}
	}

	// Step 1: Generate SSH keys for deploy
	if err := s.setupSSHKeys(srv); err != nil {
		return err
	}

	// Step 2: Update system packages
	if err := s.setupSystemUpdate(srv, sshAvailable); err != nil {
		return err
	}

	// Step 3: Create deploy user
	if err := s.setupDeployUser(srv, sshAvailable); err != nil {
		return err
	}

	// Step 4: Install Docker
	if err := s.setupDocker(srv, sshAvailable); err != nil {
		return err
	}

	// Step 5: Install Loki plugin (optional)
	if err := s.setupLokiPlugin(srv, sshAvailable); err != nil {
		return err
	}

	// Step 6: Install docker-rollout plugin
	if err := s.setupDockerRollout(srv, sshAvailable); err != nil {
		return err
	}

	// Step 7: Setup Docker registry login
	if err := s.setupDockerRegistryLogin(srv, sshAvailable); err != nil {
		return err
	}

	// Step 8: Create directory structure
	if err := s.setupDirectories(srv, sshAvailable); err != nil {
		return err
	}

	// Step 9: Setup nginx and certbot for public services
	publicApps := s.getPublicApps()
	if len(publicApps) > 0 {
		if err := s.setupNginx(srv, publicApps, sshAvailable); err != nil {
			return err
		}
	}

	return nil
}

// checkSSHAccessDetailed verifies SSH connectivity and returns detailed error info
func (s *Setup) checkSSHAccessDetailed(srv ServerConfig) (SSHCheckResult, bool, error) {
	port := srv.SSHPort
	if port == 0 {
		port = defaultSSHPortNum
	}

	// First, check without StrictHostKeyChecking=no to detect host key issues
	//nolint:gosec // SSH command requires user-provided host
	cmd := exec.Command(sshCommand,
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf(formatUserAtHost, srv.SSHUser, srv.Host),
		sshTestCommand,
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return SSHCheckOK, true, nil
	}

	outputStr := string(output)

	// Check for host key changed error
	if strings.Contains(outputStr, "REMOTE HOST IDENTIFICATION HAS CHANGED") ||
		strings.Contains(outputStr, "Host key verification failed") {
		return s.handleHostKeyChanged(srv, port)
	}

	// Check for authentication errors
	if strings.Contains(outputStr, "Permission denied") ||
		strings.Contains(outputStr, "Authentication failed") {
		return SSHCheckAuthFailed, false, nil
	}

	// Check for connection errors
	if strings.Contains(outputStr, "Connection refused") ||
		strings.Contains(outputStr, "Connection timed out") ||
		strings.Contains(outputStr, "No route to host") {
		return SSHCheckConnectionFailed, false, nil
	}

	return SSHCheckUnknownError, false, nil
}

// handleHostKeyChanged handles the case when SSH host key has changed
func (s *Setup) handleHostKeyChanged(srv ServerConfig, port int) (SSHCheckResult, bool, error) {
	PrintError("SSH host key has changed for " + srv.Host)
	fmt.Println("\nThis could mean:")
	fmt.Println("  1. The server was reinstalled")
	fmt.Println("  2. The IP was reassigned to a different server")
	fmt.Println("  3. A man-in-the-middle attack (unlikely but possible)")

	var removeKey bool

	removePrompt := &survey.Confirm{
		Message: fmt.Sprintf("Remove old host key for %s from known_hosts and retry?", srv.Host),
		Default: false,
		Help:    "Only do this if you're sure the server was legitimately changed",
	}

	if err := survey.AskOne(removePrompt, &removeKey); err != nil {
		return SSHCheckHostKeyChanged, false, err
	}

	if !removeKey {
		PrintInfo("SSH setup skipped. Please resolve the host key issue manually:")
		fmt.Printf("  %s -R %s\n", sshKeygenCommand, srv.Host)

		return SSHCheckHostKeyChanged, false, nil
	}

	// Remove the old key
	//nolint:gosec // ssh-keygen with user-provided host is intentional
	removeCmd := exec.Command(sshKeygenCommand, "-R", srv.Host)
	if err := removeCmd.Run(); err != nil {
		PrintError(fmt.Sprintf("Failed to remove old host key: %v", err))

		return SSHCheckHostKeyChanged, false, nil
	}

	PrintSuccess("Old host key removed from known_hosts")

	// Now try to connect again, this time accepting the new key
	var acceptNewKey bool

	acceptPrompt := &survey.Confirm{
		Message: "Accept the new host key and continue?",
		Default: true,
	}

	if err := survey.AskOne(acceptPrompt, &acceptNewKey); err != nil {
		return SSHCheckHostKeyChanged, false, err
	}

	if !acceptNewKey {
		return SSHCheckHostKeyChanged, false, nil
	}

	// Connect with StrictHostKeyChecking=accept-new to accept the new key
	//nolint:gosec // SSH command requires user-provided host
	retryCmd := exec.Command(sshCommand,
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf(formatUserAtHost, srv.SSHUser, srv.Host),
		sshTestCommand,
	)

	if err := retryCmd.Run(); err != nil {
		PrintError(fmt.Sprintf("Still cannot connect to %s: %v", srv.Host, err))

		return SSHCheckUnknownError, false, nil
	}

	PrintSuccess("New host key accepted, SSH connection verified")

	return SSHCheckOK, true, nil
}

// setupSSHKeys generates SSH keys for deploy
func (s *Setup) setupSSHKeys(srv ServerConfig) error {
	projectName := s.GetProjectName()
	keyPath := filepath.Join(os.Getenv(envHome), sshDir, fmt.Sprintf("%s_%s_deploy_id_rsa", projectName, srv.Name))

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		PrintInfo(fmt.Sprintf("SSH key already exists: %s", keyPath))

		// Ask if user wants to upload existing key to GitHub
		return s.askUploadExistingKeyToGitHub(keyPath)
	}

	// Ask user what to do with SSH key
	var keyChoice string

	keyPrompt := &survey.Select{
		Message: fmt.Sprintf("SSH deploy key for server '%s':", srv.Name),
		Options: []string{
			"Generate new key",
			"Use existing key",
			"Skip (configure manually later)",
		},
		Default: "Generate new key",
	}

	if err := survey.AskOne(keyPrompt, &keyChoice); err != nil {
		return err
	}

	switch keyChoice {
	case "Use existing key":
		return s.useExistingSSHKey(srv, keyPath)
	case "Skip (configure manually later)":
		PrintInfo("Skipping SSH key. Configure SSH_PRIVATE_KEY manually in CI/CD secrets.")

		return nil
	}

	// Generate new key
	return s.generateAndUploadSSHKey(keyPath)
}

// generateAndUploadSSHKey generates a new SSH key and uploads it to GitHub
func (s *Setup) generateAndUploadSSHKey(keyPath string) error {
	commands := []string{
		fmt.Sprintf(`ssh-keygen -q -t rsa -f %s -N ''`, keyPath),
	}

	PrintInfo("Generating SSH deploy keys...")

	if s.DryRun {
		fmt.Printf("[DRY-RUN] Would generate SSH key: %s\n", keyPath)

		return nil
	}

	// Generate key locally
	cmd := exec.Command(sshKeygenCommand, "-q", "-t", "rsa", "-f", keyPath, "-N", "")
	if err := cmd.Run(); err != nil {
		PrintManualInstructions("Generate SSH key manually:", commands)

		return nil
	}

	PrintSuccess(fmt.Sprintf("SSH key generated: %s", keyPath))

	// Read and display public key
	pubKeyPath := keyPath + pubKeyExtension
	pubKey, err := os.ReadFile(pubKeyPath)

	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	fmt.Printf("\nPublic key (will be added to server's authorized_keys):\n")
	fmt.Println(string(pubKey))

	// Upload private key to GitHub automatically
	return s.uploadSSHKeyToGitHub(keyPath)
}

// uploadSSHKeyToGitHub uploads the private SSH key to GitHub as SSH_PRIVATE_KEY secret
func (s *Setup) uploadSSHKeyToGitHub(keyPath string) error {
	// Check if gh CLI is available
	if !isCommandAvailable(ghCLI) {
		PrintWarning("GitHub CLI (gh) not found. Please upload SSH_PRIVATE_KEY manually:")
		fmt.Printf(formatSSHKeyUploadCmd, s.SetupConfig.CI.Repo, keyPath)

		return nil
	}

	// Check if CI repo is configured
	if s.SetupConfig.CI.Repo == "" {
		PrintWarning("CI repository not configured. Please upload SSH_PRIVATE_KEY manually after running 'setup ci'")

		return nil
	}

	// Read private key
	privKey, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	PrintInfo("Uploading SSH private key to GitHub...")

	// Upload to GitHub
	//nolint:gosec // gh CLI with user-provided repo is intentional
	cmd := exec.Command(ghCLI, "secret", "set", "SSH_PRIVATE_KEY", "-R", s.SetupConfig.CI.Repo, "--body", string(privKey))
	if err := cmd.Run(); err != nil {
		PrintError(fmt.Sprintf("Failed to upload SSH key to GitHub: %v", err))
		PrintInfo("Please upload manually:")
		fmt.Printf(formatSSHKeyUploadCmd, s.SetupConfig.CI.Repo, keyPath)

		return nil
	}

	PrintSuccess("SSH_PRIVATE_KEY uploaded to GitHub secrets")

	return nil
}

// askUploadExistingKeyToGitHub asks if user wants to upload existing key to GitHub
func (s *Setup) askUploadExistingKeyToGitHub(keyPath string) error {
	// Check if gh CLI is available
	if !isCommandAvailable(ghCLI) {
		return nil
	}

	// Check if CI repo is configured
	if s.SetupConfig.CI.Repo == "" {
		return nil
	}

	var upload bool

	uploadPrompt := &survey.Confirm{
		Message: "Upload this SSH key to GitHub as SSH_PRIVATE_KEY secret?",
		Default: true,
	}

	if err := survey.AskOne(uploadPrompt, &upload); err != nil {
		return err
	}

	if upload {
		return s.uploadSSHKeyToGitHub(keyPath)
	}

	return nil
}

// useExistingSSHKey prompts for an existing key path and copies it
func (s *Setup) useExistingSSHKey(_ ServerConfig, targetKeyPath string) error {
	// Get default SSH directory
	defaultSSHDir := filepath.Join(os.Getenv(envHome), sshDir)

	// List existing keys in ~/.ssh
	var existingKeys []string

	files, err := os.ReadDir(defaultSSHDir)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), pubKeyExtension) {
				// Add the private key name (without .pub)
				keyName := strings.TrimSuffix(f.Name(), pubKeyExtension)
				existingKeys = append(existingKeys, filepath.Join(defaultSSHDir, keyName))
			}
		}
	}

	if len(existingKeys) == 0 {
		PrintWarning("No existing SSH keys found in ~/.ssh")
		PrintInfo("Please configure SSH_PRIVATE_KEY manually in CI/CD secrets.")

		return nil
	}

	// Add option for custom path
	existingKeys = append(existingKeys, optionCustomPath)

	var selectedKey string

	keyPathPrompt := &survey.Select{
		Message: "Select existing SSH key:",
		Options: existingKeys,
	}

	if err := survey.AskOne(keyPathPrompt, &selectedKey); err != nil {
		return err
	}

	// Handle custom path
	if selectedKey == optionCustomPath {
		customPrompt := &survey.Input{
			Message: "Enter path to private key:",
			Default: filepath.Join(defaultSSHDir, "id_rsa"),
		}

		if err := survey.AskOne(customPrompt, &selectedKey); err != nil {
			return err
		}
	}

	// Verify key exists
	if _, err := os.Stat(selectedKey); os.IsNotExist(err) {
		PrintError(fmt.Sprintf("Key not found: %s", selectedKey))

		return nil
	}

	// Copy key to target path
	privKey, err := os.ReadFile(selectedKey)
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	if err := os.WriteFile(targetKeyPath, privKey, filePermPrivate); err != nil {
		return fmt.Errorf("failed to copy key: %w", err)
	}

	// Copy public key if exists
	pubKeySource := selectedKey + pubKeyExtension
	pubKeyTarget := targetKeyPath + pubKeyExtension

	if pubKey, err := os.ReadFile(pubKeySource); err == nil {
		if err := os.WriteFile(pubKeyTarget, pubKey, filePermPublic); err != nil {
			PrintWarning(fmt.Sprintf("Failed to copy public key: %v", err))
		}
	}

	PrintSuccess(fmt.Sprintf("Using existing key: %s -> %s", selectedKey, targetKeyPath))

	// Show public key for reference
	if pubKey, err := os.ReadFile(pubKeyTarget); err == nil {
		fmt.Printf("\nPublic key (ensure it's in server's authorized_keys):\n")
		fmt.Println(string(pubKey))
	}

	// Ask if user wants to upload to GitHub
	return s.askUploadExistingKeyToGitHub(targetKeyPath)
}

// setupSystemUpdate updates system packages
func (s *Setup) setupSystemUpdate(srv ServerConfig, sshAvailable bool) error {
	commands := []string{
		`apt-get -y update`,
		`apt-get -y upgrade`,
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, "Updating system packages")
	}

	PrintManualInstructions("Update system packages on server:", commands)

	return nil
}

// setupDeployUser creates the deploy user on the server
func (s *Setup) setupDeployUser(srv ServerConfig, sshAvailable bool) error {
	projectName := s.GetProjectName()
	keyPath := filepath.Join(os.Getenv(envHome), sshDir, fmt.Sprintf("%s_%s_deploy_id_rsa.pub", projectName, srv.Name))

	pubKey := "<PUBLIC_KEY>"
	if data, err := os.ReadFile(keyPath); err == nil {
		pubKey = strings.TrimSpace(string(data))
	}

	commands := []string{
		fmt.Sprintf(`adduser --ingroup www-data --comment "GitHub deployer" --disabled-password %s`, srv.DeployUser),
		fmt.Sprintf(`cd /home/%s`, srv.DeployUser),
		`mkdir -p .ssh`,
		fmt.Sprintf(`echo '%s' > .ssh/authorized_keys`, pubKey),
		fmt.Sprintf(`chown -R %s:www-data /home/%s/.ssh`, srv.DeployUser, srv.DeployUser),
		`chmod 700 .ssh`,
		`chmod 600 .ssh/authorized_keys`,
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, "Creating deploy user")
	}

	PrintManualInstructions("Create deploy user on server:", commands)
	return nil
}

// setupDocker installs Docker on the server
func (s *Setup) setupDocker(srv ServerConfig, sshAvailable bool) error {
	commands := []string{
		`apt-get -y install ca-certificates curl`,
		`install -m 0755 -d /etc/apt/keyrings`,
		`curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc`,
		`chmod a+r /etc/apt/keyrings/docker.asc`,
		//nolint:lll // official Docker installation command
		`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] ` +
			`https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") ` +
			`stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null`,
		`apt-get update`,
		`apt-get -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin`,
		`systemctl enable docker`,
		`systemctl start docker`,
		fmt.Sprintf(`usermod -aG docker %s`, srv.DeployUser),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, "Installing Docker")
	}

	PrintManualInstructions("Install Docker on server:", commands)
	return nil
}

// setupLokiPlugin installs the Loki Docker plugin
func (s *Setup) setupLokiPlugin(srv ServerConfig, sshAvailable bool) error {
	commands := []string{
		`docker plugin install grafana/loki-docker-driver:latest --alias loki --grant-all-permissions`,
	}

	if sshAvailable && !s.DryRun {
		PrintInfo("Installing Loki Docker plugin (optional)...")
		// This might fail if already installed, that's ok
		//nolint:errcheck // Loki plugin installation is optional and may fail
		s.executeRemoteCommands(srv, commands, "Installing Loki plugin")

		return nil
	}

	PrintManualInstructions("Install Loki Docker plugin (optional):", commands)

	return nil
}

// setupDockerRollout installs the docker-rollout plugin
func (s *Setup) setupDockerRollout(srv ServerConfig, sshAvailable bool) error {
	deployUser := srv.DeployUser

	dockerRolloutURL := "https://raw.githubusercontent.com/Educentr/docker-rollout/main/docker-rollout"

	commands := []string{
		fmt.Sprintf(`sudo -u %s mkdir -p /home/%s/.docker/cli-plugins`, deployUser, deployUser),
		fmt.Sprintf(`sudo -u %s curl %s -o /home/%s/.docker/cli-plugins/docker-rollout`,
			deployUser, dockerRolloutURL, deployUser),
		fmt.Sprintf(`sudo -u %s chmod +x /home/%s/.docker/cli-plugins/docker-rollout`, deployUser, deployUser),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, "Installing docker-rollout plugin")
	}

	PrintManualInstructions("Install docker-rollout plugin:", commands)
	return nil
}

// setupDockerRegistryLogin configures Docker registry login
func (s *Setup) setupDockerRegistryLogin(srv ServerConfig, _ bool) error {
	deployUser := srv.DeployUser
	var commands []string

	switch s.SetupConfig.Registry.Type {
	case registryTypeGitHub:
		commands = []string{
			fmt.Sprintf(`echo "<GHCR_TOKEN>" | sudo -u %s docker login ghcr.io -u <GHCR_USER> --password-stdin`, deployUser),
		}
	case registryTypeDigitalOcean:
		commands = []string{
			`snap install doctl`,
			`snap connect doctl:dot-docker`,
			fmt.Sprintf(`sudo -u %s doctl registry login -t <DO_TOKEN> --read-only --never-expire`, deployUser),
		}
	case registryTypeSelfHosted:
		commands = []string{
			fmt.Sprintf(`echo "<REGISTRY_PASSWORD>" | sudo -u %s docker login %s -u <REGISTRY_USER> --password-stdin`,
				deployUser, s.SetupConfig.Registry.Server),
		}
	case registryTypeAWS:
		awsCliURL := "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip"
		commands = []string{
			`# AWS ECR login requires aws-cli and credentials`,
			fmt.Sprintf(`# Install aws-cli: curl "%s" -o "awscliv2.zip" && unzip awscliv2.zip && sudo ./aws/install`, awsCliURL),
			fmt.Sprintf(`# Login: aws ecr get-login-password --region <REGION> | `+
				`sudo -u %s docker login --username AWS --password-stdin %s`,
				deployUser, s.SetupConfig.Registry.Server),
		}
	}

	PrintManualInstructions(fmt.Sprintf("Configure Docker registry login (%s):", s.SetupConfig.Registry.Type), commands)
	return nil
}

// setupDirectories creates the directory structure for deployment
func (s *Setup) setupDirectories(srv ServerConfig, sshAvailable bool) error {
	projectName := s.GetProjectName()
	deployUser := srv.DeployUser

	commands := []string{
		fmt.Sprintf(`mkdir -p /opt/%s-cd/envs`, projectName),
		fmt.Sprintf(`chown -R %s:www-data /opt/%s-cd`, deployUser, projectName),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, "Creating directory structure")
	}

	PrintManualInstructions("Create directory structure:", commands)
	return nil
}

// setupNginx installs and configures nginx for public services
func (s *Setup) setupNginx(srv ServerConfig, apps []ApplicationConfig, sshAvailable bool) error {
	// First, prompt for DNS records
	fmt.Println("\n--- DNS Configuration Required ---")

	for _, app := range apps {
		fmt.Printf("Please create DNS A record: %s → %s\n", app.Domain, srv.Host)
	}
	fmt.Println()

	// Install nginx and certbot
	installCommands := []string{
		`apt-get -y install nginx certbot python3-certbot-nginx`,
		`systemctl enable nginx`,
		`systemctl start nginx`,
		`grep -q "certbot renew" /etc/crontab || echo "10 15 * * * root /usr/bin/certbot renew" >> /etc/crontab`,
	}

	if sshAvailable && !s.DryRun {
		if err := s.executeRemoteCommands(srv, installCommands, "Installing nginx and certbot"); err != nil {
			return err
		}
	} else {
		PrintManualInstructions("Install nginx and certbot:", installCommands)
	}

	// Configure each service
	for _, app := range apps {
		if err := s.configureNginxApp(srv, app, sshAvailable); err != nil {
			return err
		}
	}

	return nil
}

// configureNginxApp configures nginx for a single application
func (s *Setup) configureNginxApp(srv ServerConfig, app ApplicationConfig, sshAvailable bool) error {
	adminEmail := s.SetupConfig.AdminEmail

	// Calculate port from prefix (e.g., prefix 80 -> port 8080)
	port := app.PortPrefix*portMultiplier + portOffset

	// Nginx config template
	nginxConfig := fmt.Sprintf(`upstream %s {
    server localhost:%d;
}

server {
    listen 80;
    server_name %s;
    access_log /var/log/nginx/%s-access.log;
    error_log /var/log/nginx/%s-error.log;

    location / {
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://%s;
    }
}
`, app.Name, port, app.Domain, app.Domain, app.Domain, app.Name)

	commands := []string{
		fmt.Sprintf(`cat > /etc/nginx/sites-available/%s.conf << 'NGINXEOF'
%s
NGINXEOF`, app.Name, nginxConfig),
		fmt.Sprintf(`ln -sf /etc/nginx/sites-available/%s.conf /etc/nginx/sites-enabled/%s.conf`, app.Name, app.Name),
		`systemctl restart nginx`,
		fmt.Sprintf(`certbot run --nginx -d %s -m %s -n --agree-tos`, app.Domain, adminEmail),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(srv, commands, fmt.Sprintf("Configuring nginx for %s", app.Name))
	}

	PrintManualInstructions(fmt.Sprintf("Configure nginx for %s:", app.Name), commands)
	return nil
}

// executeRemoteCommands executes commands on the remote server via SSH
func (s *Setup) executeRemoteCommands(srv ServerConfig, commands []string, description string) error {
	PrintInfo(description + "...")

	port := srv.SSHPort
	if port == 0 {
		port = 22
	}

	// Join commands with && for sequential execution
	script := strings.Join(commands, " && ")

	//nolint:gosec // SSH command requires user-provided host
	cmd := exec.Command(sshCommand,
		"-o", sshOptionStrictHostKey,
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf(formatUserAtHost, srv.SSHUser, srv.Host),
		script,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		PrintError(fmt.Sprintf("Failed: %v", err))
		PrintManualInstructions("Run these commands manually:", commands)
		return nil // Don't fail the whole setup
	}

	PrintSuccess(description + " completed")
	return nil
}

// getPublicApps returns applications with domains
func (s *Setup) getPublicApps() []ApplicationConfig {
	var result []ApplicationConfig

	for _, app := range s.SetupConfig.Applications {
		if app.Domain != "" {
			result = append(result, app)
		}
	}

	return result
}
