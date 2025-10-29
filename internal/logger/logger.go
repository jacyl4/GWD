package logger

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
)

type ColoredLogger struct {
	infoColor    *color.Color  // Info level (blue)
	successColor *color.Color  // Success level (green)
	warnColor    *color.Color  // Warning level (yellow)
	errorColor   *color.Color  // Error level (red)
	debugColor   *color.Color  // Debug level (cyan)
	whiteColor   *color.Color  // Normal text (white)
}

func NewLogger() *ColoredLogger {
	return &ColoredLogger{
		infoColor:    color.New(color.FgBlue, color.Bold),
		successColor: color.New(color.FgGreen, color.Bold),
		warnColor:    color.New(color.FgYellow, color.Bold),
		errorColor:   color.New(color.FgRed, color.Bold),
		debugColor:   color.New(color.FgCyan, color.Bold),
		whiteColor:   color.New(color.FgWhite, color.Bold),
	}
}

func (l *ColoredLogger) Info(format string, args ...interface{}) {
	l.infoColor.Printf("[INFO] "+format+"\n", args...)
}

func (l *ColoredLogger) Success(format string, args ...interface{}) {
	l.successColor.Printf("[✓] "+format+"\n", args...)
}

func (l *ColoredLogger) Warn(format string, args ...interface{}) {
	l.warnColor.Printf("[!] "+format+"\n", args...)
}

func (l *ColoredLogger) Error(format string, args ...interface{}) {
	l.errorColor.Printf("[✕] "+format+"\n", args...)
}

func (l *ColoredLogger) Fatal(format string, args ...interface{}) {
	l.errorColor.Printf("[FATAL] "+format+"\n", args...)
	os.Exit(1)
}

func (l *ColoredLogger) Debug(format string, args ...interface{}) {
	l.debugColor.Printf("[DEBUG] "+format+"\n", args...)
}

func (l *ColoredLogger) White(format string, args ...interface{}) {
	l.whiteColor.Printf(format+"\n", args...)
}

func (l *ColoredLogger) Progress(operation string) {
	l.whiteColor.Printf("[...] %s\r", operation)
}

func (l *ColoredLogger) ProgressDone(operation string) {
	l.successColor.Printf("[✓] %s\n", operation)
}


func (l *ColoredLogger) StatusCheck(service string, status string) {
	var statusMark string
	var serviceLabel = l.whiteColor.Sprint(service)

	switch status {
	case "active":
		statusMark = l.successColor.Sprint("✓")
	case "inactive":
		statusMark = l.errorColor.Sprint("✕")
	case "not-installed":
		statusMark = l.warnColor.Sprint("!")
	default:
			statusMark = l.whiteColor.Sprint("-")
	}

	fmt.Printf("[ %s ] %s\n", statusMark, serviceLabel)
}


func (l *ColoredLogger) PrintBanner() {
	l.successColor.Println("============SERVER==============================================")
	l.successColor.Println("      _______       ______  ")
	l.successColor.Println("     / ____/ |     / / __ \\ ")
	l.successColor.Println("    / / __ | | /| / / / / / ")
	l.successColor.Println("   / /_/ / | |/ |/ / /_/ /  ")
	l.successColor.Println("   \\____/  |__/|__/_____/   ")
	l.successColor.Println("                            ")
	l.successColor.Println("")
	l.successColor.Println("Require: Debian (amd64 && arm64)")
	l.successColor.Println("Author:  JacyL4")
	l.successColor.Println("================================================================")
}


func (l *ColoredLogger) PrintSeparator(char string, length int) {
	separator := strings.Repeat(char, length)
	l.infoColor.Println(separator)
}


func (l *ColoredLogger) PrintNodeInfo(domain, port, uuid, path string) {
	l.PrintSeparator("-", 50)
	l.successColor.Println("Node Information")
	l.whiteColor.Println("")
	
	var domainWithPort string
	if port == "443" {
		domainWithPort = domain
	} else {
		domainWithPort = fmt.Sprintf("%s:%s", domain, port)
	}
	
	fmt.Printf("%s       %s\n", 
		l.infoColor.Sprint("DoH:"),
		l.warnColor.Sprintf("%s/dq", domainWithPort))
	fmt.Printf("%s   %s\n", 
		l.infoColor.Sprint("Address:"),
		l.warnColor.Sprint(domainWithPort))
	fmt.Printf("%s      %s\n", 
		l.infoColor.Sprint("UUID:"),
		l.warnColor.Sprint(uuid))
	fmt.Printf("%s      %s\n", 
		l.infoColor.Sprint("Path:"),
		l.warnColor.Sprint(path))
	
	l.PrintSeparator("-", 50)
}

func (l *ColoredLogger) SetStandardLogger() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[GWD] ")
}
