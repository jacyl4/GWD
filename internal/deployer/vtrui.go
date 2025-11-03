package deployer

import "path/filepath"

const (
	vtruiComponentName   = "vtrui"
	vtruiBinaryName      = "vtrui"
	vtruiServiceUnit     = "vtrui.service"
	vtruiConfigDir       = "/opt/GWD/vtrui"
	vtruiServiceTemplate = "vtrui.service"
)

// NewVtrui returns a deployable vtrui component.
func NewVtrui(repoDir string) Component {
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        vtruiComponentName,
		BinaryName:  vtruiBinaryName,
		BinaryPath:  filepath.Join(binaryDir, vtruiBinaryName),
		ServiceUnit: vtruiServiceUnit,
		Service: TemplateConfig{
			Source: vtruiServiceTemplate,
		},
		ConfigDirs: []string{vtruiConfigDir},
	})
}
