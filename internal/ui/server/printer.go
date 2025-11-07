package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// ServiceStatus represents a high level service health status.
type ServiceStatus string

const (
	StatusActive       ServiceStatus = "active"
	StatusInactive     ServiceStatus = "inactive"
	StatusNotInstalled ServiceStatus = "not-installed"
	StatusDisabled     ServiceStatus = "disabled"
	StatusUnknown      ServiceStatus = "unknown"
)

// Printer renders rich terminal UI fragments used by the CLI.
type Printer struct {
	colorEnabled bool
	success      *color.Color
	info         *color.Color
	warn         *color.Color
	error        *color.Color
	plain        *color.Color
}

// NewPrinter constructs a Printer with colour automatically enabled for TTY outputs.
func NewPrinter() *Printer {
	enabled := supportsColor(os.Stdout) && os.Getenv("NO_COLOR") == ""

	p := &Printer{
		colorEnabled: enabled,
		success:      color.New(color.FgGreen, color.Bold),
		info:         color.New(color.FgBlue, color.Bold),
		warn:         color.New(color.FgYellow, color.Bold),
		error:        color.New(color.FgRed, color.Bold),
		plain:        color.New(color.Reset),
	}

	if !enabled {
		p.success.DisableColor()
		p.info.DisableColor()
		p.warn.DisableColor()
		p.error.DisableColor()
		p.plain.DisableColor()
	}

	return p
}

// PrintBanner renders the application banner.
func (p *Printer) PrintBanner() {
	lines := []string{
		"============SERVER=======================================",
		"      _______       ______  ",
		"     / ____/ |     / / __ \\ ",
		"    / / __ | | /| / / / / / ",
		"   / /_/ / | |/ |/ / /_/ /  ",
		"   \\____/  |__/|__/_____/   ",
		"                            ",
		"",
		"Require: Debian (amd64 && arm64)",
		"Author:  JacyL4",
		"=========================================================",
	}

	for _, line := range lines {
		p.success.Println(line)
	}
}

// PrintSeparator prints a repeated character separator.
func (p *Printer) PrintSeparator(char string, length int) {
	if length <= 0 {
		return
	}
	fmt.Println(strings.Repeat(char, length))
}

// NodeInfo summarises the connection parameters of a node.
type NodeInfo struct {
	Domain string
	Port   string
	UUID   string
	Path   string
}

// PrintNodeInfo renders node metadata in a user friendly way.
func (p *Printer) PrintNodeInfo(info NodeInfo) {
	p.PrintSeparator("-", 50)
	p.success.Println("Node Information")
	fmt.Println()

	domainWithPort := info.Domain
	if info.Port != "" && info.Port != "443" {
		domainWithPort = fmt.Sprintf("%s:%s", info.Domain, info.Port)
	}

	fmt.Printf("%s       %s\n",
		p.info.Sprint("DoH:"),
		p.warn.Sprintf("%s/dq", domainWithPort))
	fmt.Printf("%s   %s\n",
		p.info.Sprint("Address:"),
		p.warn.Sprint(domainWithPort))
	fmt.Printf("%s      %s\n",
		p.info.Sprint("UUID:"),
		p.warn.Sprint(info.UUID))
	fmt.Printf("%s      %s\n",
		p.info.Sprint("Path:"),
		p.warn.Sprint(info.Path))

	p.PrintSeparator("-", 50)
}

// PrintServiceStatus renders the service status indicator line.
func (p *Printer) PrintServiceStatus(service string, status ServiceStatus) {
	var (
		mark string
		text string
	)

	switch status {
	case StatusActive:
		mark = p.success.Sprint("✓")
		text = "active"
	case StatusInactive:
		mark = p.error.Sprint("✕")
		text = "inactive"
	case StatusNotInstalled:
		mark = p.warn.Sprint("!")
		text = "not installed"
	case StatusDisabled:
		mark = p.warn.Sprint("!")
		text = "disabled"
	default:
		mark = "-"
		text = "unknown"
	}

	fmt.Printf("[ %s ] %s (%s)\n", mark, service, text)
}

func supportsColor(w *os.File) bool {
	return term.IsTerminal(int(w.Fd()))
}
