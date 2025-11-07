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
	templatesSrcDir := filepath.Join(repoDir, "templates_tcsss")
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        tcsssComponentName,
		BinaryName:  tcsssBinaryName,
		BinaryPath:  filepath.Join(binaryDir, tcsssBinaryName),
		ServiceUnit: tcsssServiceUnit,
		Service: TemplateConfig{
			Source: tcsssServiceTemplate,
		},
		PostInstall: func() error {
			if err := copyDirectory(templatesSrcDir, "/etc/tcsss"); err != nil {
				return err
			}
			if err := systemctlRestart(tcsssServiceUnit); err != nil {
				return err
			}
			if err := systemctlEnable(tcsssServiceUnit); err != nil {
				return err
			}
			return nil
		},
	})
}
