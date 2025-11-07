package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	configserver "GWD/internal/configurator/server"
	"GWD/internal/deployer"
	serverdownloader "GWD/internal/downloader/server"
	apperrors "GWD/internal/errors"
	errorlog "GWD/internal/errors/logging"
	"GWD/internal/logger"
	dpkg "GWD/internal/pkgmgr"
	"GWD/internal/system"
	timesync "GWD/internal/timesync"
	ui "GWD/internal/ui/server"
)

type Installer struct {
	sysConfig *system.Config
	console   *ui.Console
	logger    logger.Logger

	installConfig *InstallConfig

	validator  *EnvironmentValidator
	pkgManager *dpkg.Manager
	repository *serverdownloader.Downloader
	doh        deployer.Component
	nginx      deployer.Component
	vtrui      deployer.Component
	tcsss      deployer.Component
}

const (
	defaultWebSocketPath = "/ws"
	defaultNginxConfDir  = "/etc/nginx/conf.d"
	defaultCertPath      = "/var/www/ssl/de_GWD.cer"
	defaultKeyPath       = "/var/www/ssl/de_GWD.key"
	defaultDHParamPath   = "/var/www/ssl/dhparam.pem"
)

// NewInstaller creates a new Installer instance. Package manager is constructed here
// to keep server wiring minimal.
func NewInstaller(
	cfg *system.Config,
	console *ui.Console,
	repo *serverdownloader.Downloader,
	validator *EnvironmentValidator,
) *Installer {
	return &Installer{
		sysConfig:  cfg,
		console:    console,
		logger:     console.Logger(),
		validator:  validator,
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
func (i *Installer) InstallGWD(cfg *InstallConfig) error {
	ctx := context.Background()
	i.installConfig = cfg
	defer func() { i.installConfig = nil }()

	setupSteps := []InstallStep{
		{
			Name:      "Validate install configuration",
			Operation: "installer.validateInstallConfig",
			Category:  apperrors.ErrCategoryValidation,
			Fn: func() error {
				if cfg == nil {
					return apperrors.New(
						apperrors.ErrCategoryValidation,
						apperrors.CodeValidationGeneric,
						"install configuration is required",
						nil,
					)
				}
				return cfg.Validate()
			},
		},
		{
			Name:      "Validate system configuration",
			Operation: "installer.validateSystemConfiguration",
			Category:  apperrors.ErrCategoryValidation,
			Fn:        i.sysConfig.Validate,
		},
		{
			Name:      "Create working directories",
			Operation: "installer.createWorkingDirectories",
			Category:  apperrors.ErrCategorySystem,
			Fn:        i.createWorkingDirectories,
		},
		{
			Name:      "Check runtime environment",
			Operation: "installer.validateEnvironment",
			Category:  apperrors.ErrCategorySystem,
			Fn:        i.validator.Validate,
		},
	}

	setupPipeline := NewPipeline(i.console, i.logger, setupSteps, i.pipelineErrorHandler(ctx))
	if err := setupPipeline.Execute(); err != nil {
		return err
	}

	installSteps := []InstallStep{
		{"Upgrade system packages", "installer.upgradeSystemPackages", apperrors.ErrCategoryDependency, i.pkgManager.UpgradeSystem},
		{"Install system dependencies", "installer.installDependencies", apperrors.ErrCategoryDependency, i.pkgManager.InstallDependencies},
		{"Set timezone to Asia/Shanghai", "installer.configureTimezone", apperrors.ErrCategorySystem, configserver.EnsureTimezoneShanghai},
		{"Configure rng-tools and chrony", "installer.configureEntropyAndTime", apperrors.ErrCategorySystem, i.configureEntropyAndTime},
		{"Configure unbound", "installer.configureUnbound", apperrors.ErrCategorySystem, i.configureUnbound},
		{"Configure resolvconf", "installer.configureResolvconf", apperrors.ErrCategorySystem, configserver.EnsureResolvconfConfig},
		{"Synchronize system time", "installer.syncTime", apperrors.ErrCategorySystem, i.syncSystemTime},
		{"Download repository files", "installer.downloadRepository", apperrors.ErrCategoryDependency, i.repository.DownloadAll},
		{"Generate SSL certificate", "installer.generateSSLCertificate", apperrors.ErrCategoryDeployment, func() error { return i.generateSSLCertificate(cfg) }},
		{"Install tcsss", "installer.installTcsss", apperrors.ErrCategoryDeployment, i.installTcsss},
		{"Install DoH server", "installer.installDoH", apperrors.ErrCategoryDeployment, i.installDOHServer},
		{"Install vtrui", "installer.installVtrui", apperrors.ErrCategoryDeployment, i.installVtrui},
		{"Install Nginx", "installer.installNginx", apperrors.ErrCategoryDeployment, i.installNginx},
		{"Start system services", "installer.startSystemServices", apperrors.ErrCategoryDeployment, i.startSystemServices},
		{"Configure SSL certificate", "installer.configureTLS", apperrors.ErrCategoryDeployment, func() error { return i.configureTLS(cfg) }},
		{"Configure Nginx Web", "installer.configureNginxWeb", apperrors.ErrCategoryDeployment, i.configureNginxWeb},
		{"Post-installation configuration", "installer.postInstall", apperrors.ErrCategoryDeployment, i.postInstallConfiguration},
	}

	installPipeline := NewPipeline(i.console, i.logger, installSteps, i.pipelineErrorHandler(ctx))
	if err := installPipeline.Execute(); err != nil {
		return err
	}

	i.console.Success("GWD installation completed")
	return nil
}

// createWorkingDirectories creates the working directories required by GWD
func (i *Installer) createWorkingDirectories() error {
	const dirPerm os.FileMode = 0o755

	type dirSpec struct {
		path string
		desc string
	}

	seen := make(map[string]struct{})

	addDir := func(path, desc string) error {
		if path == "" {
			return nil
		}
		cleanPath := filepath.Clean(path)
		if _, exists := seen[cleanPath]; exists {
			return nil
		}
		seen[cleanPath] = struct{}{}

		if err := os.MkdirAll(cleanPath, dirPerm); err != nil {
			return i.wrapError(
				apperrors.ErrCategorySystem,
				"installer.createWorkingDirectories",
				fmt.Sprintf("failed to create %s", desc),
				err,
				apperrors.Metadata{"path": cleanPath},
			)
		}
		i.logger.Debug("Created directory: %s", cleanPath)
		return nil
	}

	addDirGroup := func(basePath, baseDesc string, children ...dirSpec) error {
		if err := addDir(basePath, baseDesc); err != nil {
			return err
		}
		for _, child := range children {
			childPath := child.path
			if !filepath.IsAbs(childPath) {
				childPath = filepath.Join(basePath, childPath)
			}
			if err := addDir(childPath, child.desc); err != nil {
				return err
			}
		}
		return nil
	}

	if err := addDir(i.sysConfig.WorkingDir, "Main working directory"); err != nil {
		return err
	}
	if err := addDirGroup(
		i.sysConfig.GetRepoDir(),
		"Repository files directory",
		dirSpec{path: "templates_tcsss", desc: "TCSSS templates directory"},
	); err != nil {
		return err
	}
	if err := addDir(i.sysConfig.GetLogDir(), "Log directory"); err != nil {
		return err
	}
	if err := addDir("/var/www/html", "Web root directory"); err != nil {
		return err
	}
	if err := addDirGroup(
		"/var/www/ssl",
		"SSL certificate directory",
		dirSpec{path: ".acme.sh", desc: "ACME working directory"},
	); err != nil {
		return err
	}
	if err := addDirGroup(
		"/etc/nginx",
		"Nginx configuration root",
		dirSpec{path: "conf.d", desc: "Nginx configuration directory"},
		dirSpec{path: "stream.d", desc: "Nginx stream configuration directory"},
	); err != nil {
		return err
	}
	if err := addDir("/var/log/nginx", "Nginx log directory"); err != nil {
		return err
	}
	if err := addDirGroup(
		"/var/cache/nginx",
		"Nginx cache root",
		dirSpec{path: "client_temp", desc: "Nginx client cache directory"},
		dirSpec{path: "proxy_temp", desc: "Nginx proxy cache directory"},
		dirSpec{path: "fastcgi_temp", desc: "Nginx FastCGI cache directory"},
		dirSpec{path: "scgi_temp", desc: "Nginx SCGI cache directory"},
		dirSpec{path: "uwsgi_temp", desc: "Nginx uWSGI cache directory"},
	); err != nil {
		return err
	}
	if err := addDir("/etc/tcsss", "TCSSS configuration directory"); err != nil {
		return err
	}

	return nil
}

func (i *Installer) syncSystemTime() error {
	i.logger.Info("Synchronizing system time...")
	result, err := timesync.Sync(context.Background(), nil)
	if err != nil {
		return i.wrapError(apperrors.ErrCategorySystem, "installer.syncSystemTime", "Time synchronization failed", err, nil)
	}
	if result != nil {
		i.logger.Info("System time synchronized using %s", result.Source)
	}
	return nil
}

// installDOHServer installs the DoH (DNS-over-HTTPS) server
func (i *Installer) installDOHServer() error {
	i.logger.Info("Configuring DoH server...")
	if err := i.doh.Install(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installDoH", "DoH deployment failed", err, nil)
	}
	if err := i.doh.Validate(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installDoH", "DoH validation failed", err, nil)
	}

	return nil
}

// installNginx installs and configures Nginx
func (i *Installer) installNginx() error {
	i.logger.Info("Configuring Nginx server...")
	if err := i.nginx.Install(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installNginx", "Nginx deployment failed", err, nil)
	}
	if err := i.nginx.Validate(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installNginx", "Nginx validation failed", err, nil)
	}
	return nil
}

// installXray installs and configures the Xray proxy
// Xray support removed

// installTcsss installs and configures the tcsss service
func (i *Installer) installTcsss() error {
	i.logger.Info("Configuring tcsss service...")
	if err := i.tcsss.Install(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installTcsss", "tcsss deployment failed", err, nil)
	}
	if err := i.tcsss.Validate(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installTcsss", "tcsss validation failed", err, nil)
	}
	return nil
}

// installVtrui installs and configures the vtrui service
func (i *Installer) installVtrui() error {
	i.logger.Info("Configuring vtrui service...")
	if err := i.vtrui.Install(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installVtrui", "vtrui deployment failed", err, nil)
	}
	if err := i.vtrui.Validate(); err != nil {
		return i.wrapError(apperrors.ErrCategoryDeployment, "installer.installVtrui", "vtrui validation failed", err, nil)
	}
	return nil
}

func (i *Installer) configureEntropyAndTime() error {
	i.logger.Info("Configuring rng-tools and chrony...")
	if err := configserver.EnsureEntropyAndTimeConfigured(); err != nil {
		return err
	}

	if err := i.ensureServiceEnabled(configserver.RngToolsServiceCandidates()); err != nil {
		return err
	}
	if err := i.ensureServiceRestarted(configserver.RngToolsServiceCandidates()); err != nil {
		return err
	}
	if err := i.ensureServiceEnabled(configserver.ChronyServiceCandidates()); err != nil {
		return err
	}
	if err := i.ensureServiceRestarted(configserver.ChronyServiceCandidates()); err != nil {
		return err
	}

	return nil
}

func (i *Installer) configureUnbound() error {
	i.logger.Info("Configuring unbound service...")
	if err := configserver.EnsureUnboundConfig(); err != nil {
		return err
	}

	if err := i.systemctlDaemonReload(); err != nil {
		return err
	}
	if err := i.systemctlEnable("unbound"); err != nil {
		return err
	}
	if err := i.systemctlRestart("unbound"); err != nil {
		return err
	}

	return nil
}

func (i *Installer) generateSSLCertificate(cfg *InstallConfig) error {
	if cfg == nil || cfg.TLS == nil {
		return i.wrapError(
			apperrors.ErrCategoryConfig,
			"installer.generateSSLCertificate",
			"TLS configuration is required for certificate generation",
			nil,
			nil,
		)
	}

	domain := strings.TrimSpace(cfg.Domain)
	if domain == "" {
		return i.wrapError(
			apperrors.ErrCategoryValidation,
			"installer.generateSSLCertificate",
			"domain is required for certificate generation",
			errors.New("empty domain"),
			nil,
		)
	}

	input := domain
	if cfg.Port != 443 {
		input = fmt.Sprintf("%s:%d", domain, cfg.Port)
	}

	opts := configserver.ACMECertificateOptions{Domain: input}

	if cfg.TLS.Provider == TLSProviderCloudflare {
		opts.CloudflareEmail = strings.TrimSpace(cfg.TLS.Email)
		opts.CloudflareKey = strings.TrimSpace(cfg.TLS.APIKey)
	}

	i.logger.Info("Generating SSL certificate for %s...", domain)
	if err := configserver.EnsureACMECertificate(opts); err != nil {
		i.logger.Warn("Failed to generate SSL certificate for %s: %v", domain, err)
		return nil
	}

	return nil
}

// startSystemServices starts all deployed system services in the correct order.
func (i *Installer) startSystemServices() error {
	i.logger.Info("Starting system services...")

	services := []struct {
		name        string
		displayName string
	}{
		{"doh-server.service", "DoH server"},
		{"tcsss.service", "TCSSS"},
		{"nginx.service", "Nginx"},
		{"vtrui.service", "Vtrui"},
	}

	for _, svc := range services {
		i.logger.Info("Configuring %s service...", svc.displayName)
		if err := i.startAndEnableService(svc.name); err != nil {
			return i.wrapError(
				apperrors.ErrCategoryDeployment,
				"installer.startSystemServices",
				fmt.Sprintf("failed to start %s service", svc.displayName),
				err,
				apperrors.Metadata{"service": svc.name},
			)
		}
	}

	i.logger.Info("All system services started successfully")
	return nil
}

// startAndEnableService enables and starts a systemd service.
func (i *Installer) startAndEnableService(serviceName string) error {
	if err := i.systemctlDaemonReload(); err != nil {
		return err
	}

	i.logger.Info("Enabling %s...", serviceName)
	if err := i.systemctlEnable(serviceName); err != nil {
		return err
	}

	i.logger.Info("Starting %s...", serviceName)
	if err := i.systemctlRestart(serviceName); err != nil {
		return err
	}

	i.logger.Info("Service %s started successfully", serviceName)
	return nil
}

func (i *Installer) ensureServiceEnabled(candidates []string) error {
	return i.applyServiceAction(candidates, i.systemctlEnable, "enable")
}

func (i *Installer) ensureServiceRestarted(candidates []string) error {
	return i.applyServiceAction(candidates, i.systemctlRestart, "restart")
}

func (i *Installer) applyServiceAction(candidates []string, action func(string) error, actionName string) error {
	if len(candidates) == 0 {
		return i.wrapError(
			apperrors.ErrCategorySystem,
			"installer.applyServiceAction",
			"no service candidates provided",
			nil,
			apperrors.Metadata{"action": actionName},
		)
	}

	var lastErr error
	for _, svc := range candidates {
		if err := action(svc); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return lastErr
}

// systemctlDaemonReload reloads systemd daemon configuration.
func (i *Installer) systemctlDaemonReload() error {
	cmd := exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return i.wrapError(
			apperrors.ErrCategorySystem,
			"installer.systemctlDaemonReload",
			"failed to reload systemd daemon",
			err,
			apperrors.Metadata{"output": string(output)},
		)
	}
	return nil
}

// systemctlRestart restarts a systemd service.
func (i *Installer) systemctlRestart(serviceName string) error {
	cmd := exec.Command("systemctl", "restart", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return i.wrapError(
			apperrors.ErrCategorySystem,
			"installer.systemctlRestart",
			"failed to restart service",
			err,
			apperrors.Metadata{
				"service": serviceName,
				"output":  string(output),
			},
		)
	}
	return nil
}

// systemctlEnable enables a systemd service.
func (i *Installer) systemctlEnable(serviceName string) error {
	cmd := exec.Command("systemctl", "enable", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return i.wrapError(
			apperrors.ErrCategorySystem,
			"installer.systemctlEnable",
			"failed to enable service",
			err,
			apperrors.Metadata{
				"service": serviceName,
				"output":  string(output),
			},
		)
	}
	return nil
}

// configureTLS configures TLS/SSL certificates
func (i *Installer) configureTLS(cfg *InstallConfig) error {
	i.logger.Info("Configuring SSL/TLS certificates...")

	if cfg == nil || cfg.TLS == nil {
		return i.wrapError(
			apperrors.ErrCategoryConfig,
			"installer.configureTLS",
			"TLS configuration is missing",
			nil,
			nil,
		)
	}

	metadata := apperrors.Metadata{
		"provider": cfg.TLS.Provider,
		"domain":   cfg.Domain,
		"port":     cfg.Port,
	}

	switch cfg.TLS.Provider {
	case TLSProviderLetsEncrypt:
		return i.wrapError(
			apperrors.ErrCategoryDeployment,
			"installer.configureTLS",
			"Let's Encrypt integration not yet implemented",
			nil,
			metadata,
		)
	case TLSProviderCloudflare:
		return i.wrapError(
			apperrors.ErrCategoryDeployment,
			"installer.configureTLS",
			"Cloudflare SSL integration not yet implemented",
			nil,
			metadata,
		)
	default:
		return i.wrapError(
			apperrors.ErrCategoryConfig,
			"installer.configureTLS",
			"Unknown TLS provider",
			nil,
			metadata,
		)
	}
}

// configureNginxWeb configures Nginx Web service
func (i *Installer) configureNginxWeb() error {
	cfg := i.installConfig
	if cfg == nil {
		return i.wrapError(
			apperrors.ErrCategoryConfig,
			"installer.configureNginxWeb",
			"install configuration not available",
			nil,
			nil,
		)
	}

	domain := strings.TrimSpace(cfg.Domain)
	i.logger.Info("Configuring Nginx web service for %s...", domain)

	options := configserver.NginxOptions{
		Port:        cfg.Port,
		Domain:      domain,
		ConfigDir:   defaultNginxConfDir,
		WSPath:      defaultWebSocketPath,
		CertFile:    defaultCertPath,
		KeyFile:     defaultKeyPath,
		DHParamFile: defaultDHParamPath,
	}

	if err := configserver.EnsureNginxConfig(options); err != nil {
		return i.wrapError(
			apperrors.ErrCategoryDeployment,
			"installer.configureNginxWeb",
			"failed to configure Nginx web service",
			err,
			apperrors.Metadata{
				"domain":  domain,
				"port":    cfg.Port,
				"ws_path": options.WSPath,
			},
		)
	}

	return nil
}

// postInstallConfiguration post-installation configuration
func (i *Installer) postInstallConfiguration() error {
	i.logger.Info("Performing post-installation configuration...")
	return i.wrapError(
		apperrors.ErrCategoryDeployment,
		"installer.postInstall",
		"Post-install configuration not yet implemented",
		nil,
		nil,
	)
}

func (i *Installer) pipelineErrorHandler(ctx context.Context) StepErrorHandler {
	return func(step InstallStep, err error) error {
		appErr := i.wrapError(
			step.Category,
			step.Operation,
			fmt.Sprintf("%s failed", step.Name),
			err,
			apperrors.Metadata{"step": step.Name},
		)
		errorlog.Error(ctx, i.logger, fmt.Sprintf("%s failed", step.Name), appErr)
		return appErr
	}
}

func (i *Installer) wrapError(category apperrors.ErrorCategory, operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	if err == nil {
		return apperrors.New(category, errorCodeForCategory(category), message, nil).
			WithModule("installer").
			WithOperation(operation).
			WithFields(metadata)
	}

	if appErr, ok := apperrors.As(err); ok {
		if appErr.Module == "" {
			appErr.WithModule("installer")
		}
		if operation != "" && appErr.Operation == "" {
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

	return apperrors.New(category, errorCodeForCategory(category), message, err).
		WithModule("installer").
		WithOperation(operation).
		WithFields(metadata)
}

func errorCodeForCategory(category apperrors.ErrorCategory) string {
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
