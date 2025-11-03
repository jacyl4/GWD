package deployer

import "path/filepath"

const (
	tcsssComponentName   = "tcsss"
	tcsssBinaryName      = "tcsss"
	tcsssServiceUnit     = "tcsss.service"
	tcsssServiceTemplate = "tcsss.service"
)

// NewTcsss returns a deployable tcsss component.
func NewTcsss(repoDir string) Component {
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        tcsssComponentName,
		BinaryName:  tcsssBinaryName,
		BinaryPath:  filepath.Join(binaryDir, tcsssBinaryName),
		ServiceUnit: tcsssServiceUnit,
		Service: TemplateConfig{
			Source: tcsssServiceTemplate,
		},
	})
}
