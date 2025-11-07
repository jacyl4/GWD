package menu

// MenuOption represents a selectable option shown to the user.
type MenuOption struct {
	Label       string
	Description string
	Handler     func() error
	Color       string
	Enabled     bool
}

// DomainInfo captures the user supplied domain configuration.
type DomainInfo struct {
	Domain           string
	TopDomain        string
	Port             string
	CloudflareConfig *CloudflareConfig
}

// CloudflareConfig stores Cloudflare API credentials for certificate automation.
type CloudflareConfig struct {
	APIKey string
	Email  string
}
