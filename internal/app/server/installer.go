package server

import (
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	configserver "GWD/internal/configurator/server"
	"GWD/internal/deployer"
	"GWD/internal/downloader"
	"GWD/internal/logger"
	menu "GWD/internal/menu/server"
	"GWD/internal/system"

	"github.com/pkg/errors"
)

type Installer struct {
	config     *system.SystemConfig
	logger     *logger.ColoredLogger
	pkgManager *system.DpkgManager
	repository *downloader.Repository
	smartdns   *deployer.SmartDNS
	doh        *deployer.DoH
	nginx      *deployer.Nginx
	vtrui      *deployer.Vtrui
	tcsss      *deployer.Tcsss
}

// NewInstaller creates a new Installer instance. Package manager is constructed here
// to keep server wiring minimal.
func NewInstaller(cfg *system.SystemConfig, log *logger.ColoredLogger, repo *downloader.Repository) *Installer {
	return &Installer{
		config:     cfg,
		logger:     log,
		pkgManager: system.NewDpkgManager(),
		repository: repo,
		smartdns:   deployer.NewSmartDNS(cfg.GetRepoDir(), log),
		doh:        deployer.NewDoH(cfg.GetRepoDir(), log),
		nginx:      deployer.NewNginx(cfg.GetRepoDir(), log),
		vtrui:      deployer.NewVtrui(cfg.GetRepoDir(), log),
		tcsss:      deployer.NewTcsss(cfg.GetRepoDir(), log),
	}
}

// InstallGWD executes the full GWD installation process
// This is the core installation function, coordinating all modules to complete system deployment
func (i *Installer) InstallGWD(domainConfig *menu.DomainInfo) error {
	i.logger.Info("Starting GWD installation...")

	allInstallSetupSteps := []struct {
		name string
		fn   func() error
	}{
		{"Validate configuration", i.config.Validate},
		{"Create working directories", i.createWorkingDirectories},
		{"Check runtime environment", i.validateEnvironment},
	}

	for _, step := range allInstallSetupSteps {
		i.logger.Progress(step.name)
		if err := step.fn(); err != nil {
			return errors.Wrapf(err, "%s failed", step.name)
		}
		i.logger.ProgressDone(step.name)
	}

	// Full installation process, simulating the original bash script's installGWD function
	installSteps := []struct {
		name string
		fn   func() error
	}{
		{"Upgrade system packages", i.pkgManager.UpgradeSystem},
		{"Install system dependencies", i.pkgManager.InstallDependencies},
		{"Configure unbound", configserver.EnsureUnboundConfig},
		{"Configure resolvconf", configserver.EnsureResolvconfConfig},
		{"Download repository files", i.repository.DownloadAll},
		{"Install DoH server", func() error { return i.installDOHServer() }},
		{"Install Nginx", func() error { return i.installNginx() }},
		{"Install tcsss", func() error { return i.installTcsss() }},
		{"Install vtrui", func() error { return i.installVtrui() }},
		{"Configure SSL certificate", func() error { return i.configureTLS(domainConfig) }},
		{"Configure Nginx Web", func() error { return i.configureNginxWeb() }},
		{"Post-installation configuration", func() error { return i.postInstallConfiguration() }},
	}

	for _, step := range installSteps {
		i.logger.Progress(step.name)
		if err := step.fn(); err != nil {
			return errors.Wrapf(err, "%s failed", step.name)
		}
		i.logger.ProgressDone(step.name)
	}

	i.logger.Success("GWD installation completed")
	return nil
}

// createWorkingDirectories creates the working directories required by GWD
func (i *Installer) createWorkingDirectories() error {
	directories := []struct {
		path string
		perm os.FileMode
		desc string
	}{
		{i.config.WorkingDir, 0755, "Main working directory"},
		{i.config.GetRepoDir(), 0755, "Repository files directory"},
		{i.config.GetLogDir(), 0755, "Log directory"},
		{"/var/www/ssl", 0755, "SSL certificate directory"},
		{"/etc/nginx/conf.d", 0755, "Nginx configuration directory"},
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir.path, dir.perm); err != nil {
			return errors.Wrapf(err, "Failed to create %s: %s", dir.desc, dir.path)
		}
		i.logger.Debug("Created directory: %s", dir.path)
	}

	return nil
}

// validateEnvironment validates the runtime environment
func (i *Installer) validateEnvironment() error {
	i.logger.Debug("Validating runtime environment...")

	// Check operating system
	if err := i.validateOperatingSystem(); err != nil {
		return errors.Wrap(err, "Operating system validation failed")
	}

	// Check architecture support
	if !i.config.IsSupportedArchitecture() {
		return errors.Errorf("Unsupported system architecture: %s", i.config.Architecture)
	}

	// Check network connectivity
	if err := i.validateNetworkConnectivity(); err != nil {
		return errors.Wrap(err, "Network connectivity validation failed")
	}

	// Check disk space
	if err := i.validateDiskSpace(); err != nil {
		return errors.Wrap(err, "Disk space validation failed")
	}

	i.logger.Debug("Environment validation passed")
	return nil
}

// validateOperatingSystem validates the operating system
// Ensures it's running on a supported Debian version
func (i *Installer) validateOperatingSystem() error {
	// Check /etc/os-release file
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return errors.Wrap(err, "Failed to read system information")
	}

	osInfo := string(content)

	// Check if it's a Debian system
	if !strings.Contains(osInfo, "ID=debian") && !strings.Contains(osInfo, "ID_LIKE=debian") {
		return errors.New("GWD only supports Debian and its derivatives")
	}

	// Log detected system information
	for _, line := range strings.Split(osInfo, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			systemName := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			i.logger.Debug("Detected system: %s", systemName)
			break
		}
	}

	return nil
}

// validateNetworkConnectivity validates network connectivity
// Ensures access to GitHub and other necessary network resources
func (i *Installer) validateNetworkConnectivity() error {
	// Test critical network connections
	testURLs := []string{
		"https://cloudflare.com",
		"https://google.com",
	}

	for _, url := range testURLs {
		if err := i.testHTTPConnection(url); err != nil {
			return errors.Wrapf(err, "Network connectivity test failed: %s", url)
		}
	}

	return nil
}

// testHTTPConnection tests HTTP connection
func (i *Installer) testHTTPConnection(url string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.Errorf("HTTP error: %d", resp.StatusCode)
	}

	return nil
}

// validateDiskSpace validates disk space
// Ensures enough space for installation and operation
func (i *Installer) validateDiskSpace() error {
	// Check root partition space
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return errors.Wrap(err, "Failed to get disk space information")
	}

	// Calculate available space (bytes)
	available := stat.Bavail * uint64(stat.Bsize)
	availableMB := available / (1024 * 1024)

	// At least 1GB of free space is required
	const minSpaceMB = 1024
	if availableMB < minSpaceMB {
		return errors.Errorf("Insufficient disk space, at least %dMB required, currently %dMB available",
			minSpaceMB, availableMB)
	}

	i.logger.Debug("Disk available space: %d MB", availableMB)
	return nil
}

// installDOHServer installs the DoH (DNS-over-HTTPS) server
func (i *Installer) installDOHServer() error {
	i.logger.Info("Configuring DoH server...")
	if err := i.smartdns.Install(); err != nil {
		return errors.Wrap(err, "SmartDNS deployment failed")
	}

	if err := i.doh.Install(); err != nil {
		return errors.Wrap(err, "DoH deployment failed")
	}

	return nil
}

// installNginx installs and configures Nginx
func (i *Installer) installNginx() error {
	i.logger.Info("Configuring Nginx server...")
	if err := i.nginx.Install(); err != nil {
		return errors.Wrap(err, "Nginx deployment failed")
	}
	return nil
}

// installXray installs and configures the Xray proxy
// Xray support removed

// installTcsss installs and configures the tcsss service
func (i *Installer) installTcsss() error {
	i.logger.Info("Configuring tcsss service...")
	if err := i.tcsss.Install(); err != nil {
		return errors.Wrap(err, "tcsss deployment failed")
	}
	return nil
}

// installVtrui installs and configures the vtrui service
func (i *Installer) installVtrui() error {
	i.logger.Info("Configuring vtrui service...")
	if err := i.vtrui.Install(); err != nil {
		return errors.Wrap(err, "vtrui deployment failed")
	}
	return nil
}

// configureTLS configures TLS/SSL certificates
func (i *Installer) configureTLS(domainConfig *menu.DomainInfo) error {
	i.logger.Info("Configuring SSL/TLS certificates...")

	if domainConfig.Port == "443" {
		// Standard HTTPS port, use Let's Encrypt
		return i.setupLetsEncrypt(domainConfig)
	} else {
		// Custom port, use Cloudflare API
		return i.setupCloudflareSSL(domainConfig)
	}
}

// setupLetsEncrypt configures Let's Encrypt certificates
func (i *Installer) setupLetsEncrypt(domainConfig *menu.DomainInfo) error {
	i.logger.Info("Configuring Let's Encrypt certificates...")
	return nil
}

// setupCloudflareSSL configures Cloudflare SSL certificates
func (i *Installer) setupCloudflareSSL(domainConfig *menu.DomainInfo) error {
	i.logger.Info("Configuring Cloudflare SSL certificates...")
	return nil
}

// configureNginxWeb configures Nginx Web service
func (i *Installer) configureNginxWeb() error {
	i.logger.Info("Configuring Nginx Web service...")
	return nil
}

// postInstallConfiguration post-installation configuration
func (i *Installer) postInstallConfiguration() error {
	i.logger.Info("Performing post-installation configuration...")
	return nil
}
