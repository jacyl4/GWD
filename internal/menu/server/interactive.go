package menu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"GWD/internal/logger"
	"GWD/internal/system"
	ui "GWD/internal/ui/server"

	"github.com/manifoldco/promptui"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/pkg/errors"
)

// Menu is the interactive menu manager
// Responsible for displaying menus, handling user input, and calling corresponding functional modules
type Menu struct {
	config         *system.Config
	console        *ui.Console
	logger         logger.Logger
	printer        *ui.Printer
	installHandler func(*DomainInfo) error
}

// NewMenu creates a new menu manager instance
func NewMenu(cfg *system.Config, console *ui.Console) *Menu {
	var log logger.Logger
	if console != nil {
		log = console.Logger()
	}
	if log == nil {
		log = logger.NewStandardLogger()
	}

	return &Menu{
		config:  cfg,
		console: console,
		logger:  log,
		printer: ui.NewPrinter(),
	}
}

// SetInstallHandler registers the handler that executes the full installation workflow.
func (m *Menu) SetInstallHandler(handler func(*DomainInfo) error) {
	m.installHandler = handler
}

// MenuOption defines a menu option
// Each option includes a display label, description, and corresponding handler function
type MenuOption struct {
	Label       string       // Display label
	Description string       // Detailed description
	Handler     func() error // Handler function
	Color       string       // Display color (green, red, yellow, cyan)
	Enabled     bool         // Whether the option is enabled
}

// ShowMainMenu displays the main menu
// This is the Go implementation of the original bash script's start_menu function
// Displays different menu options based on the system environment (container vs. physical machine)
func (m *Menu) ShowMainMenu() error {
	for {
		// Clear screen and display system status
		m.clearScreen()
		if err := m.displaySystemStatus(); err != nil {
			m.logger.Error("Failed to display system status: %v", err)
		}

		// Display banner
		m.printer.PrintBanner()

		// Build menu options based on environment type
		var options []MenuOption
		if m.config.IsContainer() {
			options = m.buildContainerMenu()
		} else {
			options = m.buildStandardMenu()
		}

		// Display menu and get user selection
		selected, err := m.promptUserSelection(options)
		if err != nil {
			if err.Error() == "^C" {
				// User pressed Ctrl+C to exit
				m.logger.Info("User cancelled operation")
				return nil
			}
			return errors.Wrap(err, "Failed to process user input")
		}

		// Execute the selected operation
		if err := options[selected].Handler(); err != nil {
			m.logger.Error("Operation failed: %v", err)
			m.waitForUserInput("\nPress Enter to continue...")
		}
	}
}

// buildContainerMenu builds menu options for container environments
// Container environments do not support kernel installation and some system-level operations
func (m *Menu) buildContainerMenu() []MenuOption {
	return []MenuOption{
		{
			Label:       "1. Install GWD",
			Description: "Fresh installation of GWD reverse proxy system",
			Handler:     m.handleInstallGWD,
			Color:       "green",
			Enabled:     true,
		},
	}
}

// buildStandardMenu builds menu options for standard environments (physical/virtual machines)
// Includes a full set of features, including kernel installation and advanced network functions
func (m *Menu) buildStandardMenu() []MenuOption {
	return []MenuOption{
		{
			Label:       "1. Install GWD",
			Description: "Fresh installation of GWD reverse proxy system",
			Handler:     m.handleInstallGWD,
			Color:       "green",
			Enabled:     true,
		},
	}
}

// promptUserSelection displays menu options and gets user selection
// Uses the promptui library to provide a modern interactive experience
func (m *Menu) promptUserSelection(options []MenuOption) (int, error) {
	// Build list of display items, only including enabled options
	var items []string
	var enabledIndexes []int

	numberPattern := regexp.MustCompile(`^(\d+)\.\s*(.*)$`)

	type menuEntry struct {
		prefix        string
		numberPart    string
		textPart      string
		originalIndex int
	}

	var entries []menuEntry
	maxNumberWidth := 0

	for i, option := range options {
		if !option.Enabled {
			continue
		}

		var prefix string
		switch option.Color {
		case "red":
			prefix = "🔴"
		case "green":
			prefix = "🟢"
		case "yellow":
			prefix = "🟡"
		case "cyan":
			prefix = "🔵"
		default:
			prefix = "⚪"
		}

		numberPart := ""
		textPart := option.Label
		if matches := numberPattern.FindStringSubmatch(option.Label); len(matches) == 3 {
			numberPart = matches[1]
			textPart = matches[2]

			if width := len(numberPart); width > maxNumberWidth {
				maxNumberWidth = width
			}
		}

		entries = append(entries, menuEntry{
			prefix:        prefix,
			numberPart:    numberPart,
			textPart:      textPart,
			originalIndex: i,
		})
	}

	// Determine prefix width for alignment
	maxPrefixWidth := 0
	for _, entry := range entries {
		if width := runewidth.StringWidth(entry.prefix); width > maxPrefixWidth {
			maxPrefixWidth = width
		}
	}

	numberColumnWidth := 0
	if maxNumberWidth > 0 {
		numberColumnWidth = maxNumberWidth + 2 // account for ". "
	}

	for _, entry := range entries {
		prefixDisplay := entry.prefix + strings.Repeat(" ", maxPrefixWidth-runewidth.StringWidth(entry.prefix))

		numberColumn := ""
		if entry.numberPart != "" && maxNumberWidth > 0 {
			numberField := fmt.Sprintf("%*s", maxNumberWidth, entry.numberPart)
			numberColumn = fmt.Sprintf("%s. ", numberField)
		} else if numberColumnWidth > 0 {
			numberColumn = strings.Repeat(" ", numberColumnWidth)
		}

		items = append(items, fmt.Sprintf("%s %s%s", prefixDisplay, numberColumn, entry.textPart))
		enabledIndexes = append(enabledIndexes, entry.originalIndex)
	}

	// Create selection prompt
	prompt := promptui.Select{
		Label:             "Please select an operation",
		Items:             items,
		Size:              10, // Display 10 options, supports scrolling
		HideHelp:          false,
		StartInSearchMode: false,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}:",
			Active:   "▶ {{ . | cyan }}",
			Inactive: "  {{ . }}",
			Selected: "✅ {{ . | green }}",
			Help:     "{{ \"Navigate:\" | faint }} {{ .NextKey }} {{ .PrevKey }} {{ .PageDownKey }} {{ .PageUpKey }} {{ \"|\" | faint }} {{ \"Exit:\" | faint }} Ctrl + C",
		},
	}

	index, _, err := prompt.Run()
	if err != nil {
		return -1, err
	}

	// Return the actual option index
	if index >= 0 && index < len(enabledIndexes) {
		return enabledIndexes[index], nil
	}

	return -1, errors.New("Invalid selection")
}

// displaySystemStatus displays system status information
// Includes service status, version information, environment information, etc.
func (m *Menu) displaySystemStatus() error {
	// Get SSL certificate expiration date
	sslExpireDate := m.getSSLExpireDate()

	// Display service status
	m.displayServiceStatus()

	// Display environment information
	m.displayEnvironmentInfo(sslExpireDate)

	return nil
}

// displayServiceStatus displays the running status of key services
func (m *Menu) displayServiceStatus() {
	services := map[string]string{
		"Nginx":      "nginx",
		"Xray":       "vtrui",
		"DoH Server": "doh-server",
		"AutoUpdate": "", // Special handling
	}

	for displayName, serviceName := range services {
		if serviceName == "" {
			// Special handling for auto-update status
			status := "disabled"
			m.printer.PrintServiceStatus(displayName, mapServiceStatus(status))
		} else {
			status := m.getServiceStatus(serviceName)
			m.printer.PrintServiceStatus(displayName, mapServiceStatus(status))
		}
	}
}

func mapServiceStatus(status string) ui.ServiceStatus {
	switch status {
	case "active":
		return ui.StatusActive
	case "inactive":
		return ui.StatusInactive
	case "not-installed":
		return ui.StatusNotInstalled
	case "disabled":
		return ui.StatusDisabled
	default:
		return ui.StatusUnknown
	}
}

// displayEnvironmentInfo displays environment information
func (m *Menu) displayEnvironmentInfo(sslExpireDate string) {
	m.printer.PrintSeparator("-", 64)
	m.logger.Info("Debian Version: %s", m.getDebianVersion())
	m.logger.Info("Kernel Version: %s", m.getKernelVersion())
	m.printer.PrintSeparator("-", 64)
	m.logger.Info("SSL Certificate Expiration: %s", sslExpireDate)

	// Display special feature status
	if m.isWireGuardEnabled() {
		m.writeLine("🟣 [Enabled] Cloudflare Wireguard Upstream (WARP)")
	}

	if m.isHAProxyEnabled() {
		m.writeLine("🟣 [Enabled] HAProxy TCP Port Forwarding")
	}
}

func (m *Menu) writeLine(format string, args ...interface{}) {
	if m.console != nil {
		m.console.WriteLine(format, args...)
		return
	}
	fmt.Printf(format+"\n", args...)
}

// Handler functions - Handler functions for each menu option

// handleInstallGWD handles the Install GWD option
func (m *Menu) handleInstallGWD() error {
	m.logger.Info("Starting GWD server installation...")

	// Get domain input from user
	domain, err := m.promptDomain()
	if err != nil {
		return errors.Wrap(err, "Failed to get domain")
	}

	// Parse domain and port
	domainInfo := m.parseDomainInput(domain)

	// If non-standard port is used, Cloudflare API configuration is required
	if domainInfo.Port != "443" {
		cf, err := m.promptCloudflareConfig()
		if err != nil {
			return errors.Wrap(err, "Failed to get Cloudflare configuration")
		}
		domainInfo.CloudflareConfig = cf
	}

	m.logger.Info("Domain: %s, Port: %s", domainInfo.Domain, domainInfo.Port)

	if m.installHandler == nil {
		return errors.New("installer handler is not configured")
	}

	if err := m.installHandler(domainInfo); err != nil {
		return errors.Wrap(err, "GWD installation failed")
	}

	m.waitForUserInput("\nPress Enter to continue...")

	return nil
}

// Utility functions - Helper functions

// clearScreen clears the screen
func (m *Menu) clearScreen() {
	fmt.Print("\033[H\033[2J")
}

// promptDomain prompts the user to enter a domain
// Uses promptui to provide a friendly input interface
func (m *Menu) promptDomain() (string, error) {
	validate := func(input string) error {
		if len(input) < 3 {
			return errors.New("Domain too short")
		}
		if !strings.Contains(input, ".") {
			return errors.New("Please enter a valid domain")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "Please enter VPS domain",
		Validate: validate,
	}

	return prompt.Run()
}

// DomainInfo domain configuration information
type DomainInfo struct {
	Domain           string            // Main domain
	TopDomain        string            // Top-level domain
	Port             string            // Port number
	CloudflareConfig *CloudflareConfig // Cloudflare configuration (required for non-443 ports)
}

// CloudflareConfig Cloudflare API configuration
type CloudflareConfig struct {
	APIKey string // API Key
	Email  string // Account email
}

// parseDomainInput parses user-entered domain information
func (m *Menu) parseDomainInput(input string) *DomainInfo {
	parts := strings.Split(input, ":")
	domain := parts[0]
	port := "443"

	if len(parts) > 1 {
		// Validate if port number is numeric
		if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 && p <= 65535 {
			port = parts[1]
		}
	}

	// Extract top-level domain (for some configurations)
	domainParts := strings.Split(domain, ".")
	var topDomain string
	if len(domainParts) >= 2 {
		topDomain = fmt.Sprintf("%s.%s",
			domainParts[len(domainParts)-2],
			domainParts[len(domainParts)-1])
	}

	return &DomainInfo{
		Domain:    domain,
		TopDomain: topDomain,
		Port:      port,
	}
}

// promptCloudflareConfig prompts the user to enter Cloudflare configuration
func (m *Menu) promptCloudflareConfig() (*CloudflareConfig, error) {
	// API Key input
	apiKeyPrompt := promptui.Prompt{
		Label: "Cloudflare API Key",
		Mask:  '*', // Hide input
		Validate: func(input string) error {
			if len(input) < 10 {
				return errors.New("API Key too short")
			}
			return nil
		},
	}

	apiKey, err := apiKeyPrompt.Run()
	if err != nil {
		return nil, err
	}

	// Email input
	emailPrompt := promptui.Prompt{
		Label: "Cloudflare Email",
		Validate: func(input string) error {
			if !strings.Contains(input, "@") {
				return errors.New("Please enter a valid email address")
			}
			return nil
		},
	}

	email, err := emailPrompt.Run()
	if err != nil {
		return nil, err
	}

	return &CloudflareConfig{
		APIKey: apiKey,
		Email:  email,
	}, nil
}

// waitForUserInput waits for user input (for pausing)
func (m *Menu) waitForUserInput(message string) {
	prompt := promptui.Prompt{
		Label: message,
	}
	prompt.Run()
}

// Status check functions - Status check functions

// isGWDInstalled checks if GWD is installed
func (m *Menu) isGWDInstalled() bool {
	// Check for key binaries in either the repository cache or installation targets
	paths := []string{
		filepath.Join(m.config.GetRepoDir(), "nginx"),
		filepath.Join(m.config.GetRepoDir(), "doh-server"),
		"/usr/local/bin/nginx",
		"/usr/local/bin/doh-server",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}

	return false
}

// getServiceStatus gets systemd service status
func (m *Menu) getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		// Check if service exists
		if m.isServiceInstalled(serviceName) {
			return "inactive"
		} else {
			return "not-installed"
		}
	}

	status := strings.TrimSpace(string(output))
	if status == "active" {
		return "active"
	} else {
		return "inactive"
	}
}

// isServiceInstalled checks if a service is installed
func (m *Menu) isServiceInstalled(serviceName string) bool {
	// Check if executable exists
	switch serviceName {
	case "nginx":
		_, err := os.Stat("/usr/sbin/nginx")
		return err == nil
	case "vtrui":
		_, err := os.Stat("/opt/GWD/vtrui/vtrui")
		return err == nil
	case "doh-server":
		_, err := os.Stat("/opt/GWD/doh-server")
		return err == nil
	default:
		return false
	}
}

// getSSLExpireDate gets SSL certificate expiration date
func (m *Menu) getSSLExpireDate() string {
	certPath := "/var/www/ssl/GWD.cer"

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return "Not installed"
	}

	cmd := exec.Command("openssl", "x509", "-enddate", "-noout", "-in", certPath)
	output, err := cmd.Output()
	if err != nil {
		return "Failed to retrieve"
	}

	// Parse openssl output: notAfter=Jan 1 12:00:00 2025 GMT
	result := string(output)
	if strings.HasPrefix(result, "notAfter=") {
		return strings.TrimPrefix(result, "notAfter=")
	}

	return strings.TrimSpace(result)
}

// getDebianVersion gets Debian version
func (m *Menu) getDebianVersion() string {
	cmd := exec.Command("sh", "-c", "cat /etc/os-release | grep VERSION= | cut -d'(' -f2 | cut -d')' -f1")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(output))
}

// getKernelVersion gets kernel version
func (m *Menu) getKernelVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(output))
}

// getCurrentVersion gets current GWD version
// isWireGuardEnabled checks if WireGuard is enabled
func (m *Menu) isWireGuardEnabled() bool {
	cmd := exec.Command("systemctl", "is-active", "wg-quick@wgcf")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "active"
}

// isHAProxyEnabled checks if HAProxy is enabled
func (m *Menu) isHAProxyEnabled() bool {
	cmd := exec.Command("systemctl", "is-active", "haproxy")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "active"
}
