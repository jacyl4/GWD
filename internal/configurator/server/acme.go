package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	apperrors "GWD/internal/errors"
)

const (
	acmeInstallerURL      = "https://get.acme.sh"
	acmeHomeDir           = "/var/www/ssl/.acme.sh"
	acmeScriptName        = "acme.sh"
	certificatesOutputDir = "/var/www/ssl"
)

// ACMECertificateOptions controls how certificates are issued.
type ACMECertificateOptions struct {
	Domain          string
	CloudflareEmail string
	CloudflareKey   string
}

// EnsureACMECertificate installs acme.sh if needed and issues a certificate into /var/www/ssl.
func EnsureACMECertificate(opts ACMECertificateOptions) error {
	host, hasPort, err := validateAndParseOptions(opts)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(certificatesOutputDir, 0o755); err != nil {
		return newConfiguratorError(
			"configurator.EnsureACMECertificate",
			"failed to prepare certificate directory",
			err,
			apperrors.Metadata{"path": certificatesOutputDir},
		)
	}

	if err := ensureAcmeInstalled(); err != nil {
		return err
	}

	if hasPort {
		email := strings.TrimSpace(opts.CloudflareEmail)
		key := strings.TrimSpace(opts.CloudflareKey)
		if err := issueCertificateDNS(host, email, key); err != nil {
			return err
		}
	} else {
		if err := issueCertificateWithPort80(host); err != nil {
			return err
		}
	}

	if err := installCertificateArtifacts(host); err != nil {
		return err
	}

	return nil
}

type acmeRunner struct {
	acmePath string
	homeDir  string
}

func newAcmeRunner() (*acmeRunner, error) {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err != nil {
		return nil, newConfiguratorError(
			"configurator.newAcmeRunner",
			"acme.sh executable not found",
			err,
			apperrors.Metadata{"path": acmePath},
		)
	}

	return &acmeRunner{
		acmePath: acmePath,
		homeDir:  acmeHomeDir,
	}, nil
}

func baseAcmeEnv(additional ...string) []string {
	env := []string{
		"HOME=" + certificatesOutputDir,
		"LE_WORKING_DIR=" + acmeHomeDir,
		"ACME_HOME=" + acmeHomeDir,
	}
	env = append(env, additional...)
	return append(os.Environ(), env...)
}

func (r *acmeRunner) buildEnv(additional ...string) []string {
	return baseAcmeEnv(additional...)
}

func (r *acmeRunner) runCommand(operation string, args []string, env []string, metadata apperrors.Metadata) error {
	cmd := exec.Command(r.acmePath, args...)
	cmd.Env = env

	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if metadata == nil {
			metadata = apperrors.Metadata{}
		}
		metadata["args"] = strings.Join(args, " ")
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError(
			"configurator.acmeRunner."+operation,
			"acme.sh command failed",
			err,
			metadata,
		)
	}

	return nil
}

type port80Handler struct {
	stoppedServices []string
}

func newPort80Handler() *port80Handler {
	return &port80Handler{stoppedServices: make([]string, 0)}
}

func (h *port80Handler) preparePort80() error {
	candidates := []string{"nginx.service", "nginx", "apache2.service", "apache2"}

	for _, svc := range candidates {
		if h.isServiceRunning(svc) {
			if err := h.stopService(svc); err != nil {
				return err
			}
			h.stoppedServices = append(h.stoppedServices, svc)
		}
	}

	return nil
}

func (h *port80Handler) restoreServices() error {
	var firstErr error
	for i := len(h.stoppedServices) - 1; i >= 0; i-- {
		svc := h.stoppedServices[i]
		if err := h.startService(svc); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *port80Handler) isServiceRunning(serviceName string) bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", serviceName)
	return cmd.Run() == nil
}

func (h *port80Handler) stopService(serviceName string) error {
	cmd := exec.Command("systemctl", "stop", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return newConfiguratorError(
			"configurator.port80Handler.stopService",
			"failed to stop service for port 80 access",
			err,
			apperrors.Metadata{
				"service": serviceName,
				"output":  strings.TrimSpace(string(output)),
			},
		)
	}
	return nil
}

func (h *port80Handler) startService(serviceName string) error {
	cmd := exec.Command("systemctl", "start", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return newConfiguratorError(
			"configurator.port80Handler.startService",
			"failed to restart service after certificate issuance",
			err,
			apperrors.Metadata{
				"service": serviceName,
				"output":  strings.TrimSpace(string(output)),
			},
		)
	}
	return nil
}

func validateAndParseOptions(opts ACMECertificateOptions) (string, bool, error) {
	domainInput := strings.TrimSpace(opts.Domain)
	if domainInput == "" {
		return "", false, newConfiguratorError(
			"configurator.validateAndParseOptions",
			"domain is required",
			errors.New("empty domain"),
			nil,
		)
	}

	host, hasPort, err := splitDomainAndPort(domainInput)
	if err != nil {
		return "", false, newConfiguratorError(
			"configurator.validateAndParseOptions",
			"invalid domain input",
			err,
			apperrors.Metadata{"input": domainInput},
		)
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return "", false, newConfiguratorError(
			"configurator.validateAndParseOptions",
			"domain is required",
			errors.New("empty host"),
			nil,
		)
	}

	if hasPort {
		email := strings.TrimSpace(opts.CloudflareEmail)
		key := strings.TrimSpace(opts.CloudflareKey)
		if email == "" || key == "" {
			return "", false, newConfiguratorError(
				"configurator.validateAndParseOptions",
				"cloudflare email and key required for DNS issuance",
				errors.New("missing cloudflare credentials"),
				apperrors.Metadata{"domain": host},
			)
		}
	}

	return host, hasPort, nil
}

func ensureAcmeInstalled() error {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return newConfiguratorError(
			"configurator.ensureAcmeInstalled",
			"failed to detect acme.sh",
			err,
			apperrors.Metadata{"path": acmePath},
		)
	}

	installerPath, err := downloadAcmeInstaller()
	if err != nil {
		return err
	}
	defer os.Remove(installerPath)

	if err := installAcmeScript(installerPath); err != nil {
		return err
	}

	return nil
}

func downloadAcmeInstaller() (string, error) {
	tmpFile, err := os.CreateTemp("", "get-acme-*.sh")
	if err != nil {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"failed to create temp file",
			err,
			nil,
		)
	}

	path := tmpFile.Name()
	success := false
	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(path)
		}
	}()

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(acmeInstallerURL)
	if err != nil {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"failed to download acme installer",
			err,
			nil,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"unexpected response when downloading acme installer",
			fmt.Errorf("status %d", resp.StatusCode),
			apperrors.Metadata{"status": resp.StatusCode},
		)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"failed to save acme installer",
			err,
			nil,
		)
	}

	if err := tmpFile.Close(); err != nil {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"failed to close installer temp file",
			err,
			nil,
		)
	}

	if err := os.Chmod(path, 0o700); err != nil {
		return "", newConfiguratorError(
			"configurator.downloadAcmeInstaller",
			"failed to chmod acme installer",
			err,
			apperrors.Metadata{"path": path},
		)
	}

	success = true
	return path, nil
}

func installAcmeScript(installerPath string) error {
	if err := os.MkdirAll(acmeHomeDir, 0o755); err != nil {
		return newConfiguratorError(
			"configurator.installAcmeScript",
			"failed to prepare acme home",
			err,
			apperrors.Metadata{"path": acmeHomeDir},
		)
	}

	cmd := exec.Command("sh", installerPath, "--install", "--nocron", "--noprofile")
	cmd.Env = baseAcmeEnv()

	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		metadata := apperrors.Metadata{"installer": installerPath}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError(
			"configurator.installAcmeScript",
			"acme installer execution failed",
			err,
			metadata,
		)
	}

	return nil
}

func issueCertificateStandalone(domain string) error {
	runner, err := newAcmeRunner()
	if err != nil {
		return err
	}

	args := []string{"--issue", "--standalone", "--httpport", "80", "-d", domain}
	return runner.runCommand(
		"issueCertificateStandalone",
		args,
		runner.buildEnv(),
		apperrors.Metadata{
			"domain": domain,
			"mode":   "standalone",
		},
	)
}

func issueCertificateDNS(domain, email, key string) error {
	runner, err := newAcmeRunner()
	if err != nil {
		return err
	}

	email = strings.TrimSpace(email)
	key = strings.TrimSpace(key)
	args := []string{"--issue", "--dns", "dns_cf", "-d", domain}
	return runner.runCommand(
		"issueCertificateDNS",
		args,
		runner.buildEnv("CF_Email="+email, "CF_Key="+key),
		apperrors.Metadata{
			"domain": domain,
			"mode":   "dns_cf",
			"email":  email,
		},
	)
}

func installCertificateArtifacts(domain string) error {
	runner, err := newAcmeRunner()
	if err != nil {
		return err
	}

	paths := certificatePaths(domain)
	args := []string{
		"--install-cert",
		"-d", domain,
		"--key-file", paths.key,
		"--cert-file", paths.cert,
		"--fullchain-file", paths.fullchain,
		"--ca-file", paths.intermediate,
	}

	return runner.runCommand(
		"installCertificateArtifacts",
		args,
		runner.buildEnv(),
		apperrors.Metadata{
			"domain": domain,
			"key":    paths.key,
			"cert":   paths.cert,
		},
	)
}

func issueCertificateWithPort80(domain string) error {
	handler := newPort80Handler()
	if err := handler.preparePort80(); err != nil {
		return err
	}

	defer func() {
		_ = handler.restoreServices()
	}()

	return issueCertificateStandalone(domain)
}

type certificatePathSet struct {
	key          string
	cert         string
	fullchain    string
	intermediate string
}

func certificatePaths(domain string) certificatePathSet {
	safe := sanitizeDomainForFile(domain)
	return certificatePathSet{
		key:          filepath.Join(certificatesOutputDir, safe+".key"),
		cert:         filepath.Join(certificatesOutputDir, safe+".cer"),
		fullchain:    filepath.Join(certificatesOutputDir, safe+".fullchain.pem"),
		intermediate: filepath.Join(certificatesOutputDir, safe+".ca.cer"),
	}
}

func sanitizeDomainForFile(domain string) string {
	var b strings.Builder
	for _, r := range domain {
		switch {
		case r == '*':
			b.WriteString("wildcard")
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			// skip brackets produced by IPv6 format but keep delimiter placeholder
			if r != '[' && r != ']' {
				b.WriteRune('_')
			}
		}
	}
	safe := b.String()
	if safe == "" {
		return "domain"
	}
	return safe
}

func splitDomainAndPort(input string) (string, bool, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false, errors.New("empty domain")
	}

	colonCount := strings.Count(trimmed, ":")
	if colonCount == 0 || (colonCount > 1 && !strings.Contains(trimmed, "]")) {
		return trimmed, false, nil
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return "", false, err
	}

	if host == "" {
		return "", false, errors.New("missing host")
	}

	if port == "" {
		return "", false, errors.New("missing port")
	}

	return host, true, nil
}
