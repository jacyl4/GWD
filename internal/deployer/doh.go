package deployer

import "path/filepath"

const (
	dohComponentName   = "doh-server"
	dohBinaryName      = "doh-server"
	dohServiceUnit     = "doh-server.service"
	dohServiceTemplate = "doh-server.service.tmpl"
	dohConfigPath      = "/opt/GWD/doh/doh-server.conf"
)

type dohServiceData struct {
	ConfigPath string
}

// NewDoH returns a deployable DoH component.
func NewDoH(repoDir string) Component {
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        dohComponentName,
		BinaryName:  dohBinaryName,
		BinaryPath:  filepath.Join(binaryDir, dohBinaryName),
		ServiceUnit: dohServiceUnit,
		Service: TemplateConfig{
			Source: dohServiceTemplate,
			Data: dohServiceData{
				ConfigPath: dohConfigPath,
			},
		},
	})
}
