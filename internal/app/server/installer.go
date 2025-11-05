package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	configserver "GWD/internal/configurator/server"
	"GWD/internal/deployer"
	serverdownloader "GWD/internal/downloader/server"
	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
	menu "GWD/internal/menu/server"
	"GWD/internal/pkgmgr/dpkg"
	"GWD/internal/system"
)

type Installer struct {
	config     *system.SystemConfig
	logger     *logger.ColoredLogger
	pkgManager *dpkg.Manager
	repository *serverdownloader.Downloader
	doh        deployer.Component
	nginx      deployer.Component
	vtrui      deployer.Component
	tcsss      deployer.Component
}

// NewInstaller creates a new Installer instance. Package manager is constructed here
// to keep server wiring minimal.
func NewInstaller(cfg *system.SystemConfig, log *logger.ColoredLogger, repo *serverdownloader.Downloader) *Installer {
	return &Installer{
		config:     cfg,
		logger:     log,
		pkgManager: dpkg.NewManager(nil),
		repository: repo,
		doh:        deployer.NewDoH(cfg.GetRepoDir()),
		nginx:      deployer.NewNginx(cfg.GetRepoDir()),
		vtrui:      deployer.NewVtrui(cfg.GetRepoDir()),
		tcsss:      deployer.NewTcsss(cfg.GetRepoDir()),
	}
}

// InstallGWD executes the full GWD installation process
// This is the core installation function, coordinating all modules to complete system deployment
func (i *Installer) InstallGWD(domainConfig *menu.DomainInfo) error {
	ctx := context.Background()

	allInstallSetupSteps := []struct {
		name      string
		operation string
		category  apperrors.ErrorCategory
		fn        func() error
	}{
		{"Validate configuration", "installer.validateConfiguration", apperrors.ErrCategoryValidation, i.config.Validate},
		{"Create working directories", "installer.createWorkingDirectories", apperrors.ErrCategorySystem, i.createWorkingDirectories},
		{"Check runtime environment", "installer.validateEnvironment", apperrors.ErrCategorySystem, i.validateEnvironment},
	}

	for _, step := range allInstallSetupSteps {
		i.logger.Progress(step.name)
		if err := step.fn(); err != nil {
			appErr := normalizeInstallerError(err, step.category, step.operation, fmt.Sprintf("%s failed", step.name), apperrors.Metadata{
				"step": step.name,
			})
			i.logger.ErrorWithAppError(ctx, fmt.Sprintf("%s failed", step.name), appErr)
			return appErr
		}
		i.logger.ProgressDone(step.name)
	}

	installSteps := []struct {
		name      string
		operation string
		category  apperrors.ErrorCategory
		fn        func() error
	}{
		{"Upgrade system packages", "installer.upgradeSystemPackages", apperrors.ErrCategoryDependency, i.pkgManager.UpgradeSystem},
		{"Install system dependencies", "installer.installDependencies", apperrors.ErrCategoryDependency, i.pkgManager.InstallDependencies},
		{"Set timezone to Asia/Shanghai", "installer.configureTimezone", apperrors.ErrCategorySystem, configserver.EnsureTimezoneShanghai},
		{"Configure rng-tools and chrony", "installer.configureEntropyAndTime", apperrors.ErrCategorySystem, configserver.EnsureEntropyAndTimeConfigured},
		{"Configure unbound", "installer.configureUnbound", apperrors.ErrCategorySystem, configserver.EnsureUnboundConfig},
		{"Configure resolvconf", "installer.configureResolvconf", apperrors.ErrCategorySystem, configserver.EnsureResolvconfConfig},
		{"Download repository files", "installer.downloadRepository", apperrors.ErrCategoryDependency, i.repository.DownloadAll},
		{"Install tcsss", "installer.installTcsss", apperrors.ErrCategoryDeployment, func() error { return i.installTcsss() }},
		{"Install DoH server", "installer.installDoH", apperrors.ErrCategoryDeployment, func() error { return i.installDOHServer() }},
		{"Install Nginx", "installer.installNginx", apperrors.ErrCategoryDeployment, func() error { return i.installNginx() }},
		{"Install vtrui", "installer.installVtrui", apperrors.ErrCategoryDeployment, func() error { return i.installVtrui() }},
		{"Configure SSL certificate", "installer.configureTLS", apperrors.ErrCategoryDeployment, func() error { return i.configureTLS(domainConfig) }},
		{"Configure Nginx Web", "installer.configureNginxWeb", apperrors.ErrCategoryDeployment, func() error { return i.configureNginxWeb() }},
		{"Post-installation configuration", "installer.postInstall", apperrors.ErrCategoryDeployment, func() error { return i.postInstallConfiguration() }},
	}

	for _, step := range installSteps {
		i.logger.Progress(step.name)
		if err := step.fn(); err != nil {
			appErr := normalizeInstallerError(err, step.category, step.operation, fmt.Sprintf("%s failed", step.name), apperrors.Metadata{
				"step": step.name,
			})
			i.logger.ErrorWithAppError(ctx, fmt.Sprintf("%s failed", step.name), appErr)
			return appErr
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
			return newInstallerError(
				apperrors.ErrCategorySystem,
				fmt.Sprintf("failed to create %s", dir.desc),
				"installer.createWorkingDirectories",
				err,
				apperrors.Metadata{"path": dir.path},
			)
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
		return normalizeInstallerError(err, apperrors.ErrCategoryValidation, "installer.validateEnvironment", "operating system validation failed", nil)
	}

	// Check architecture support
	if !i.config.IsSupportedArchitecture() {
		return newInstallerError(
			apperrors.ErrCategoryValidation,
			"unsupported system architecture",
			"installer.validateEnvironment",
			nil,
			apperrors.Metadata{"architecture": i.config.Architecture},
		)
	}

	// Check network connectivity
	if err := i.validateNetworkConnectivity(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryNetwork, "installer.validateEnvironment", "network connectivity validation failed", nil)
	}

	// Check disk space
	if err := i.validateDiskSpace(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategorySystem, "installer.validateEnvironment", "disk space validation failed", nil)
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
		return newInstallerError(
			apperrors.ErrCategorySystem,
			"failed to read system information",
			"installer.validateOperatingSystem",
			err,
			apperrors.Metadata{"path": "/etc/os-release"},
		)
	}

	osInfo := string(content)

	// Check if it's a Debian system
	if !strings.Contains(osInfo, "ID=debian") && !strings.Contains(osInfo, "ID_LIKE=debian") {
		return newInstallerError(
			apperrors.ErrCategoryValidation,
			"unsupported operating system",
			"installer.validateOperatingSystem",
			nil,
			apperrors.Metadata{"os_release": osInfo},
		)
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
			return normalizeInstallerError(err, apperrors.ErrCategoryNetwork, "installer.validateNetworkConnectivity", "network connectivity test failed", apperrors.Metadata{
				"url": url,
			})
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
		return newInstallerError(
			apperrors.ErrCategoryNetwork,
			"failed to establish HTTP connection",
			"installer.testHTTPConnection",
			err,
			apperrors.Metadata{"url": url},
		)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return newInstallerError(
			apperrors.ErrCategoryNetwork,
			"received unsuccessful HTTP status",
			"installer.testHTTPConnection",
			nil,
			apperrors.Metadata{
				"url":         url,
				"status_code": resp.StatusCode,
			},
		)
	}

	return nil
}

// validateDiskSpace validates disk space
// Ensures enough space for installation and operation
func (i *Installer) validateDiskSpace() error {
	// Check root partition space
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return newInstallerError(
			apperrors.ErrCategorySystem,
			"failed to get disk space information",
			"installer.validateDiskSpace",
			err,
			apperrors.Metadata{"path": "/"},
		)
	}

	// Calculate available space (bytes)
	available := stat.Bavail * uint64(stat.Bsize)
	availableMB := available / (1024 * 1024)

	// At least 1GB of free space is required
	const minSpaceMB = 1024
	if availableMB < minSpaceMB {
		return newInstallerError(
			apperrors.ErrCategorySystem,
			"insufficient disk space",
			"installer.validateDiskSpace",
			nil,
			apperrors.Metadata{
				"required_mb":  minSpaceMB,
				"available_mb": availableMB,
			},
		)
	}

	i.logger.Debug("Disk available space: %d MB", availableMB)
	return nil
}

// installDOHServer installs the DoH (DNS-over-HTTPS) server
func (i *Installer) installDOHServer() error {
	i.logger.Info("Configuring DoH server...")
	if err := i.doh.Install(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installDoH", "DoH deployment failed", nil)
	}
	if err := i.doh.Validate(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installDoH", "DoH validation failed", nil)
	}

	return nil
}

// installNginx installs and configures Nginx
func (i *Installer) installNginx() error {
	i.logger.Info("Configuring Nginx server...")
	if err := i.nginx.Install(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installNginx", "Nginx deployment failed", nil)
	}
	if err := i.nginx.Validate(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installNginx", "Nginx validation failed", nil)
	}
	return nil
}

// installXray installs and configures the Xray proxy
// Xray support removed

// installTcsss installs and configures the tcsss service
func (i *Installer) installTcsss() error {
	i.logger.Info("Configuring tcsss service...")
	if err := i.tcsss.Install(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installTcsss", "tcsss deployment failed", nil)
	}
	if err := i.tcsss.Validate(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installTcsss", "tcsss validation failed", nil)
	}
	return nil
}

// installVtrui installs and configures the vtrui service
func (i *Installer) installVtrui() error {
	i.logger.Info("Configuring vtrui service...")
	if err := i.vtrui.Install(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installVtrui", "vtrui deployment failed", nil)
	}
	if err := i.vtrui.Validate(); err != nil {
		return normalizeInstallerError(err, apperrors.ErrCategoryDeployment, "installer.installVtrui", "vtrui validation failed", nil)
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

func normalizeInstallerError(err error, category apperrors.ErrorCategory, operation, message string, metadata apperrors.Metadata) *apperrors.AppError {
	if err == nil {
		return newInstallerError(category, message, operation, nil, metadata)
	}

	if appErr, ok := apperrors.As(err); ok {
		if appErr.Module == "" {
			appErr.WithModule("installer")
		}
		if appErr.Operation == "" {
			appErr.WithOperation(operation)
		}
		if metadata != nil {
			appErr.WithFields(metadata)
		}
		if appErr.Message == "" {
			appErr.Message = message
		}
		return appErr
	}

	return newInstallerError(category, message, operation, err, metadata)
}

func newInstallerError(category apperrors.ErrorCategory, message, operation string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	code := installerCodeForCategory(category)
	appErr := apperrors.New(code, category, message, err).
		WithModule("installer").
		WithOperation(operation)
	if metadata != nil {
		appErr.WithFields(metadata)
	}
	return appErr
}

func installerCodeForCategory(category apperrors.ErrorCategory) string {
	switch category {
	case apperrors.ErrCategoryValidation:
		return apperrors.CodeValidationGeneric
	case apperrors.ErrCategoryDependency:
		return apperrors.CodeDependencyGeneric
	case apperrors.ErrCategoryNetwork:
		return apperrors.CodeNetworkGeneric
	case apperrors.ErrCategoryDeployment:
		return apperrors.CodeDeploymentGeneric
	case apperrors.ErrCategoryConfig:
		return apperrors.CodeConfigGeneric
	case apperrors.ErrCategoryFirewall:
		return apperrors.CodeFirewallGeneric
	case apperrors.ErrCategoryDatabase:
		return apperrors.CodeDatabaseGeneric
	default:
		return apperrors.CodeSystemGeneric
	}
}
