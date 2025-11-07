package menu

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/manifoldco/promptui"
	runewidth "github.com/mattn/go-runewidth"
)

func (m *Menu) promptUserSelection(options []MenuOption) (int, error) {
	items, indexes := formatMenuItems(options)

	prompt := promptui.Select{
		Label:             "Please select an operation",
		Items:             items,
		Size:              10,
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

	if index >= 0 && index < len(indexes) {
		return indexes[index], nil
	}

	return -1, errors.New("invalid selection")
}

func formatMenuItems(options []MenuOption) ([]string, []int) {
	entries := buildMenuEntries(options)
	if len(entries) == 0 {
		return nil, nil
	}

	maxPrefixWidth := 0
	maxNumberWidth := 0
	for _, entry := range entries {
		if width := runewidth.StringWidth(entry.prefix); width > maxPrefixWidth {
			maxPrefixWidth = width
		}
		if len(entry.numberPart) > maxNumberWidth {
			maxNumberWidth = len(entry.numberPart)
		}
	}

	items := make([]string, 0, len(entries))
	indexes := make([]int, 0, len(entries))

	for _, entry := range entries {
		prefix := entry.prefix + strings.Repeat(" ", maxPrefixWidth-runewidth.StringWidth(entry.prefix))

		numberColumn := ""
		if entry.numberPart != "" {
			numberColumn = fmt.Sprintf("%*s. ", maxNumberWidth, entry.numberPart)
		} else if maxNumberWidth > 0 {
			numberColumn = strings.Repeat(" ", maxNumberWidth+2)
		}

		items = append(items, fmt.Sprintf("%s %s%s", prefix, numberColumn, entry.textPart))
		indexes = append(indexes, entry.originalIndex)
	}

	return items, indexes
}

type menuEntry struct {
	prefix        string
	numberPart    string
	textPart      string
	originalIndex int
}

func buildMenuEntries(options []MenuOption) []menuEntry {
	entries := make([]menuEntry, 0, len(options))
	numberPattern := regexp.MustCompile(`^(\d+)\.\s*(.*)$`)

	for idx, option := range options {
		if !option.Enabled {
			continue
		}

		prefix := statusPrefix(option.Color)
		numberPart := ""
		textPart := option.Label

		if matches := numberPattern.FindStringSubmatch(option.Label); len(matches) == 3 {
			numberPart = matches[1]
			textPart = matches[2]
		}

		entries = append(entries, menuEntry{
			prefix:        prefix,
			numberPart:    numberPart,
			textPart:      textPart,
			originalIndex: idx,
		})
	}

	return entries
}

func statusPrefix(color string) string {
	switch color {
	case "red":
		return "🔴"
	case "green":
		return "🟢"
	case "yellow":
		return "🟡"
	case "cyan":
		return "🔵"
	default:
		return "⚪"
	}
}

func (m *Menu) promptDomain() (string, error) {
	validate := func(input string) error {
		if len(input) < 3 {
			return errors.New("domain too short")
		}
		if !strings.Contains(input, ".") {
			return errors.New("please enter a valid domain")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "Please enter VPS domain",
		Validate: validate,
	}

	return prompt.Run()
}

func (m *Menu) promptCloudflareConfig() (*CloudflareConfig, error) {
	apiKeyPrompt := promptui.Prompt{
		Label: "Cloudflare API Key",
		Mask:  '*',
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

	emailPrompt := promptui.Prompt{
		Label: "Cloudflare Email",
		Validate: func(input string) error {
			if !strings.Contains(input, "@") {
				return errors.New("please enter a valid email address")
			}
			return nil
		},
	}

	email, err := emailPrompt.Run()
	if err != nil {
		return nil, err
	}

	return &CloudflareConfig{APIKey: apiKey, Email: email}, nil
}

func (m *Menu) waitForUserInput(message string) {
	prompt := promptui.Prompt{Label: message}
	_, _ = prompt.Run()
}

func (m *Menu) parseDomainInput(input string) *DomainInfo {
	parts := strings.Split(input, ":")
	domain := parts[0]
	port := "443"

	if len(parts) > 1 {
		if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 && p <= 65535 {
			port = parts[1]
		}
	}

	domainParts := strings.Split(domain, ".")
	var topDomain string
	if len(domainParts) >= 2 {
		topDomain = fmt.Sprintf("%s.%s", domainParts[len(domainParts)-2], domainParts[len(domainParts)-1])
	}

	return &DomainInfo{
		Domain:    domain,
		TopDomain: topDomain,
		Port:      port,
	}
}
