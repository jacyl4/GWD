package deployer

import "path/filepath"

const (
	tcsssComponentName   = "tcsss"
	tcsssBinaryName      = "tcsss"
	tcsssServiceUnit     = "tcsss.service"
	tcsssServiceTemplate = "tcsss.service"
	tcsssConfigDir       = "/etc/tcsss"
)

// NewTcsss returns a deployable tcsss component.
func NewTcsss(repoDir string) Component {
	templatesSrcDir := filepath.Join(repoDir, "templates_tcsss")
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        tcsssComponentName,
		BinaryName:  tcsssBinaryName,
		BinaryPath:  filepath.Join(binaryDir, tcsssBinaryName),
		ServiceUnit: tcsssServiceUnit,
		Service: TemplateConfig{
			Source: tcsssServiceTemplate,
		},
		ConfigDirs: []string{tcsssConfigDir},
		PostInstall: func() error {
			return copyDirectory(templatesSrcDir, tcsssConfigDir)
		},
	})
}
