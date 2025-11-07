package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	validator  *EnvironmentValidator
	pkgManager *dpkg.Manager
	repository *serverdownloader.Downloader
	doh        deployer.Component
	nginx      deployer.Component
	vtrui      deployer.Component
	tcsss      deployer.Component
}

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
		{"Configure rng-tools and chrony", "installer.configureEntropyAndTime", apperrors.ErrCategorySystem, configserver.EnsureEntropyAndTimeConfigured},
		{"Configure unbound", "installer.configureUnbound", apperrors.ErrCategorySystem, configserver.EnsureUnboundConfig},
		{"Configure resolvconf", "installer.configureResolvconf", apperrors.ErrCategorySystem, configserver.EnsureResolvconfConfig},
		{"Synchronize system time", "installer.syncTime", apperrors.ErrCategorySystem, i.syncSystemTime},
		{"Download repository files", "installer.downloadRepository", apperrors.ErrCategoryDependency, i.repository.DownloadAll},
		{"Install tcsss", "installer.installTcsss", apperrors.ErrCategoryDeployment, i.installTcsss},
		{"Install DoH server", "installer.installDoH", apperrors.ErrCategoryDeployment, i.installDOHServer},
		{"Install Nginx", "installer.installNginx", apperrors.ErrCategoryDeployment, i.installNginx},
		{"Install vtrui", "installer.installVtrui", apperrors.ErrCategoryDeployment, i.installVtrui},
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
	directories := []struct {
		path string
		perm os.FileMode
		desc string
	}{
		{i.sysConfig.WorkingDir, 0755, "Main working directory"},
		{i.sysConfig.GetRepoDir(), 0755, "Repository files directory"},
		{i.sysConfig.GetLogDir(), 0755, "Log directory"},
		{"/var/www/ssl", 0755, "SSL certificate directory"},
		{"/etc/nginx/conf.d", 0755, "Nginx configuration directory"},
		{"/etc/tcsss", 0755, "TCSSS configuration directory"},
		{filepath.Join(i.sysConfig.GetRepoDir(), "templates_tcsss"), 0755, "TCSSS templates directory"},
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir.path, dir.perm); err != nil {
			return i.wrapError(
				apperrors.ErrCategorySystem,
				"installer.createWorkingDirectories",
				fmt.Sprintf("failed to create %s", dir.desc),
				err,
				apperrors.Metadata{"path": dir.path},
			)
		}
		i.logger.Debug("Created directory: %s", dir.path)
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
	i.logger.Info("Configuring Nginx Web service...")
	return i.wrapError(
		apperrors.ErrCategoryDeployment,
		"installer.configureNginxWeb",
		"Nginx web configuration not yet implemented",
		nil,
		nil,
	)
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
