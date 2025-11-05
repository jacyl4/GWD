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
	domainInput := strings.TrimSpace(opts.Domain)
	if domainInput == "" {
		return newConfiguratorError("configurator.EnsureACMECertificate", "domain is required", errors.New("empty domain"), nil)
	}

	host, hasPort, err := splitDomainAndPort(domainInput)
	if err != nil {
		return newConfiguratorError("configurator.EnsureACMECertificate", "invalid domain input", err, apperrors.Metadata{
			"input": domainInput,
		})
	}

	if err := os.MkdirAll(certificatesOutputDir, 0755); err != nil {
		return newConfiguratorError("configurator.EnsureACMECertificate", "failed to prepare certificate directory", err, apperrors.Metadata{
			"path": certificatesOutputDir,
		})
	}

	if err := ensureAcmeInstalled(); err != nil {
		return err
	}

	if hasPort {
		if strings.TrimSpace(opts.CloudflareEmail) == "" || strings.TrimSpace(opts.CloudflareKey) == "" {
			return newConfiguratorError("configurator.EnsureACMECertificate", "cloudflare email and key required for DNS issuance", errors.New("missing cloudflare credentials"), apperrors.Metadata{
				"domain": host,
			})
		}
		if err := issueCertificateDNS(host, opts.CloudflareEmail, opts.CloudflareKey); err != nil {
			return err
		}
	} else {
		if err := issueCertificateStandalone(host); err != nil {
			return err
		}
	}

	if err := installCertificateArtifacts(host); err != nil {
		return err
	}

	return nil
}

func ensureAcmeInstalled() error {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to detect acme.sh", err, apperrors.Metadata{
			"path": acmePath,
		})
	}

	tmpFile, err := os.CreateTemp("", "get-acme-*.sh")
	if err != nil {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to create temp file", err, nil)
	}
	defer os.Remove(tmpFile.Name())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(acmeInstallerURL)
	if err != nil {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to download acme installer", err, nil)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "unexpected response when downloading acme installer", fmt.Errorf("status %d", resp.StatusCode), apperrors.Metadata{
			"status": resp.StatusCode,
		})
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to save acme installer", err, nil)
	}

	if err := tmpFile.Close(); err != nil {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to close installer temp file", err, nil)
	}

	if err := os.Chmod(tmpFile.Name(), 0700); err != nil {
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "failed to chmod acme installer", err, apperrors.Metadata{
			"path": tmpFile.Name(),
		})
	}

	env := append(os.Environ(),
		"HOME="+certificatesOutputDir,
		"LE_WORKING_DIR="+acmeHomeDir,
		"ACME_HOME="+acmeHomeDir,
	)

	cmd := exec.Command("sh", tmpFile.Name(), "--install", "--nocron", "--noprofile")
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		metadata := apperrors.Metadata{
			"installer": tmpFile.Name(),
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError("configurator.EnsureAcmeInstalled", "acme installer execution failed", err, metadata)
	}

	return nil
}

func issueCertificateStandalone(domain string) error {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err != nil {
		return newConfiguratorError("configurator.IssueACMECertificate", "acme.sh executable not found", err, apperrors.Metadata{
			"path": acmePath,
		})
	}
	env := append(os.Environ(),
		"HOME="+certificatesOutputDir,
		"LE_WORKING_DIR="+acmeHomeDir,
		"ACME_HOME="+acmeHomeDir,
	)

	cmd := exec.Command(acmePath, "--issue", "--standalone", "--httpport", "80", "-d", domain)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		metadata := apperrors.Metadata{
			"domain": domain,
			"mode":   "standalone",
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError("configurator.IssueACMECertificate", "acme standalone issuance failed", err, metadata)
	}

	return nil
}

func issueCertificateDNS(domain, email, key string) error {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err != nil {
		return newConfiguratorError("configurator.IssueACMECertificate", "acme.sh executable not found", err, apperrors.Metadata{
			"path": acmePath,
		})
	}
	env := append(os.Environ(),
		"HOME="+certificatesOutputDir,
		"LE_WORKING_DIR="+acmeHomeDir,
		"ACME_HOME="+acmeHomeDir,
		"CF_Email="+strings.TrimSpace(email),
		"CF_Key="+strings.TrimSpace(key),
	)

	cmd := exec.Command(acmePath, "--issue", "--dns", "dns_cf", "-d", domain)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		metadata := apperrors.Metadata{
			"domain": domain,
			"mode":   "dns_cf",
			"email":  strings.TrimSpace(email),
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError("configurator.IssueACMECertificate", "acme dns issuance failed", err, metadata)
	}

	return nil
}

func installCertificateArtifacts(domain string) error {
	acmePath := filepath.Join(acmeHomeDir, acmeScriptName)
	if _, err := os.Stat(acmePath); err != nil {
		return newConfiguratorError("configurator.InstallACMECertificate", "acme.sh executable not found", err, apperrors.Metadata{
			"path": acmePath,
		})
	}
	env := append(os.Environ(),
		"HOME="+certificatesOutputDir,
		"LE_WORKING_DIR="+acmeHomeDir,
		"ACME_HOME="+acmeHomeDir,
	)

	paths := certificatePaths(domain)

	args := []string{
		"--install-cert",
		"-d", domain,
		"--key-file", paths.key,
		"--cert-file", paths.cert,
		"--fullchain-file", paths.fullchain,
		"--ca-file", paths.intermediate,
	}

	cmd := exec.Command(acmePath, args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		metadata := apperrors.Metadata{
			"domain": domain,
			"key":    paths.key,
			"cert":   paths.cert,
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			metadata["stderr"] = s
		}
		return newConfiguratorError("configurator.InstallACMECertificate", "failed to install acme certificates", err, metadata)
	}

	return nil
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
