package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	sshCommand             = "ssh"
	sshDir                 = ".ssh"
	sshOptionStrictHostKey = "StrictHostKeyChecking=no"
	formatUserAtHost       = "%s@%s"
)

// setupServer configures a server for the given environment
func (s *Setup) setupServer(env EnvironmentConfig) error {
	// Check SSH connectivity
	sshAvailable := s.checkSSHAccess(env)

	if sshAvailable {
		PrintSuccess(fmt.Sprintf("SSH access to %s verified", env.Server.Host))
	} else {
		PrintWarning(fmt.Sprintf("Cannot connect to %s via SSH. Will provide manual instructions.", env.Server.Host))
	}

	// Step 1: Generate SSH keys for deploy
	if err := s.setupSSHKeys(env); err != nil {
		return err
	}

	// Step 2: Create deploy user
	if err := s.setupDeployUser(env, sshAvailable); err != nil {
		return err
	}

	// Step 3: Install Docker
	if err := s.setupDocker(env, sshAvailable); err != nil {
		return err
	}

	// Step 4: Install Loki plugin (optional)
	if err := s.setupLokiPlugin(env, sshAvailable); err != nil {
		return err
	}

	// Step 5: Install docker-rollout plugin
	if err := s.setupDockerRollout(env, sshAvailable); err != nil {
		return err
	}

	// Step 6: Setup Docker registry login
	if err := s.setupDockerRegistryLogin(env, sshAvailable); err != nil {
		return err
	}

	// Step 7: Create directory structure
	if err := s.setupDirectories(env, sshAvailable); err != nil {
		return err
	}

	// Step 8: Setup nginx and certbot for public services
	publicServices := s.getPublicServicesForEnv(env)
	if len(publicServices) > 0 {
		if err := s.setupNginx(env, publicServices, sshAvailable); err != nil {
			return err
		}
	}

	return nil
}

// checkSSHAccess verifies SSH connectivity to the server
func (s *Setup) checkSSHAccess(env EnvironmentConfig) bool {
	port := env.Server.Port
	if port == 0 {
		port = 22
	}

	//nolint:gosec // SSH command requires user-provided host
	cmd := exec.Command(sshCommand,
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", sshOptionStrictHostKey,
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf(formatUserAtHost, env.Server.User, env.Server.Host),
		"echo ok",
	)

	return cmd.Run() == nil
}

// setupSSHKeys generates SSH keys for deploy
func (s *Setup) setupSSHKeys(env EnvironmentConfig) error {
	projectName := s.GetProjectName()
	keyPath := filepath.Join(os.Getenv("HOME"), sshDir, fmt.Sprintf("%s_%s_deploy_id_rsa", projectName, env.Name))

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		PrintInfo(fmt.Sprintf("SSH key already exists: %s", keyPath))
		return nil
	}

	commands := []string{
		fmt.Sprintf(`ssh-keygen -q -t rsa -f %s -N ''`, keyPath),
	}

	PrintInfo("Generating SSH deploy keys...")

	if s.DryRun {
		fmt.Printf("[DRY-RUN] Would generate SSH key: %s\n", keyPath)
		return nil
	}

	// Generate key locally
	cmd := exec.Command("ssh-keygen", "-q", "-t", "rsa", "-f", keyPath, "-N", "")
	if err := cmd.Run(); err != nil {
		PrintManualInstructions("Generate SSH key manually:", commands)
		return nil
	}

	PrintSuccess(fmt.Sprintf("SSH key generated: %s", keyPath))

	// Read public key
	pubKeyPath := keyPath + ".pub"
	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	fmt.Printf("\nPublic key (add to CI/CD SSH_PRIVATE_KEY and to server authorized_keys):\n")
	fmt.Println(string(pubKey))

	// Read private key
	privKey, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	fmt.Printf("\nPrivate key (add to CI/CD secrets as SSH_PRIVATE_KEY):\n")
	fmt.Println(string(privKey))

	return nil
}

// setupDeployUser creates the deploy user on the server
func (s *Setup) setupDeployUser(env EnvironmentConfig, sshAvailable bool) error {
	projectName := s.GetProjectName()
	keyPath := filepath.Join(os.Getenv("HOME"), sshDir, fmt.Sprintf("%s_%s_deploy_id_rsa.pub", projectName, env.Name))

	pubKey := "<PUBLIC_KEY>"
	if data, err := os.ReadFile(keyPath); err == nil {
		pubKey = strings.TrimSpace(string(data))
	}

	commands := []string{
		fmt.Sprintf(`adduser --ingroup www-data --comment "GitHub deployer" --disabled-password %s`, env.Server.DeployUser),
		fmt.Sprintf(`cd /home/%s`, env.Server.DeployUser),
		`mkdir -p .ssh`,
		fmt.Sprintf(`echo '%s' > .ssh/authorized_keys`, pubKey),
		fmt.Sprintf(`chown -R %s:www-data /home/%s/.ssh`, env.Server.DeployUser, env.Server.DeployUser),
		`chmod 700 .ssh`,
		`chmod 600 .ssh/authorized_keys`,
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(env, commands, "Creating deploy user")
	}

	PrintManualInstructions("Create deploy user on server:", commands)
	return nil
}

// setupDocker installs Docker on the server
func (s *Setup) setupDocker(env EnvironmentConfig, sshAvailable bool) error {
	commands := []string{
		`apt-get -y update`,
		`apt-get -y upgrade`,
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
		fmt.Sprintf(`usermod -aG docker %s`, env.Server.DeployUser),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(env, commands, "Installing Docker")
	}

	PrintManualInstructions("Install Docker on server:", commands)
	return nil
}

// setupLokiPlugin installs the Loki Docker plugin
func (s *Setup) setupLokiPlugin(env EnvironmentConfig, sshAvailable bool) error {
	commands := []string{
		`docker plugin install grafana/loki-docker-driver:latest --alias loki --grant-all-permissions`,
	}

	if sshAvailable && !s.DryRun {
		PrintInfo("Installing Loki Docker plugin (optional)...")
		// This might fail if already installed, that's ok
		//nolint:errcheck // Loki plugin installation is optional and may fail
		s.executeRemoteCommands(env, commands, "Installing Loki plugin")

		return nil
	}

	PrintManualInstructions("Install Loki Docker plugin (optional):", commands)

	return nil
}

// setupDockerRollout installs the docker-rollout plugin
func (s *Setup) setupDockerRollout(env EnvironmentConfig, sshAvailable bool) error {
	deployUser := env.Server.DeployUser

	dockerRolloutURL := "https://raw.githubusercontent.com/Educentr/docker-rollout/main/docker-rollout"

	commands := []string{
		fmt.Sprintf(`sudo -u %s mkdir -p /home/%s/.docker/cli-plugins`, deployUser, deployUser),
		fmt.Sprintf(`sudo -u %s curl %s -o /home/%s/.docker/cli-plugins/docker-rollout`,
			deployUser, dockerRolloutURL, deployUser),
		fmt.Sprintf(`sudo -u %s chmod +x /home/%s/.docker/cli-plugins/docker-rollout`, deployUser, deployUser),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(env, commands, "Installing docker-rollout plugin")
	}

	PrintManualInstructions("Install docker-rollout plugin:", commands)
	return nil
}

// setupDockerRegistryLogin configures Docker registry login
func (s *Setup) setupDockerRegistryLogin(env EnvironmentConfig, _ bool) error {
	deployUser := env.Server.DeployUser
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
func (s *Setup) setupDirectories(env EnvironmentConfig, sshAvailable bool) error {
	projectName := s.GetProjectName()
	deployUser := env.Server.DeployUser

	commands := []string{
		fmt.Sprintf(`mkdir -p /opt/%s-cd/envs`, projectName),
		fmt.Sprintf(`chown -R %s:www-data /opt/%s-cd`, deployUser, projectName),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(env, commands, "Creating directory structure")
	}

	PrintManualInstructions("Create directory structure:", commands)
	return nil
}

// setupNginx installs and configures nginx for public services
func (s *Setup) setupNginx(env EnvironmentConfig, services []ServiceConfig, sshAvailable bool) error {
	// First, prompt for DNS records
	fmt.Println("\n--- DNS Configuration Required ---")
	for _, svc := range services {
		fmt.Printf("Please create DNS A record: %s → %s\n", svc.Domain, env.Server.Host)
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
		if err := s.executeRemoteCommands(env, installCommands, "Installing nginx and certbot"); err != nil {
			return err
		}
	} else {
		PrintManualInstructions("Install nginx and certbot:", installCommands)
	}

	// Configure each service
	for _, svc := range services {
		if err := s.configureNginxService(env, svc, sshAvailable); err != nil {
			return err
		}
	}

	return nil
}

// configureNginxService configures nginx for a single service
func (s *Setup) configureNginxService(env EnvironmentConfig, svc ServiceConfig, sshAvailable bool) error {
	adminEmail := s.SetupConfig.AdminEmail

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
`, svc.Name, svc.PortPrefix, svc.Domain, svc.Domain, svc.Domain, svc.Name)

	commands := []string{
		fmt.Sprintf(`cat > /etc/nginx/sites-available/%s.conf << 'NGINXEOF'
%s
NGINXEOF`, svc.Name, nginxConfig),
		fmt.Sprintf(`ln -sf /etc/nginx/sites-available/%s.conf /etc/nginx/sites-enabled/%s.conf`, svc.Name, svc.Name),
		`systemctl restart nginx`,
		fmt.Sprintf(`certbot run --nginx -d %s -m %s -n --agree-tos`, svc.Domain, adminEmail),
	}

	if sshAvailable && !s.DryRun {
		return s.executeRemoteCommands(env, commands, fmt.Sprintf("Configuring nginx for %s", svc.Name))
	}

	PrintManualInstructions(fmt.Sprintf("Configure nginx for %s:", svc.Name), commands)
	return nil
}

// executeRemoteCommands executes commands on the remote server via SSH
func (s *Setup) executeRemoteCommands(env EnvironmentConfig, commands []string, description string) error {
	PrintInfo(description + "...")

	port := env.Server.Port
	if port == 0 {
		port = 22
	}

	// Join commands with && for sequential execution
	script := strings.Join(commands, " && ")

	//nolint:gosec // SSH command requires user-provided host
	cmd := exec.Command(sshCommand,
		"-o", sshOptionStrictHostKey,
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf(formatUserAtHost, env.Server.User, env.Server.Host),
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

// getPublicServicesForEnv returns services with domains for the environment
func (s *Setup) getPublicServicesForEnv(env EnvironmentConfig) []ServiceConfig {
	var result []ServiceConfig
	for _, svc := range env.Services {
		if svc.Domain != "" {
			result = append(result, svc)
		}
	}
	return result
}
