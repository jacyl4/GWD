package deployer

import "path/filepath"

const (
	smartDNSComponentName = "smartdns"
	smartDNSBinaryName    = "smartdns"
	smartDNSServiceUnit   = "smartdns.service"
	smartDNSTemplate      = "smartdns.service"
)

// NewSmartDNS returns a deployable SmartDNS component.
func NewSmartDNS(repoDir string) Component {
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        smartDNSComponentName,
		BinaryName:  smartDNSBinaryName,
		BinaryPath:  filepath.Join(binaryDir, smartDNSBinaryName),
		ServiceUnit: smartDNSServiceUnit,
		Service: TemplateConfig{
			Source: smartDNSTemplate,
		},
	})
}
