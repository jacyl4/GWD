package deployer

import "path/filepath"

const (
	nginxComponentName    = "nginx"
	nginxBinaryName       = "nginx"
	nginxServiceUnit      = "nginx.service"
	nginxServiceTemplate  = "nginx.service"
	nginxOverrideName     = "override.conf"
	nginxOverrideTemplate = "nginx-override.conf"
)

// NewNginx returns a deployable Nginx component.
func NewNginx(repoDir string) Component {
	return NewGenericDeployer(repoDir, ComponentConfig{
		Name:        nginxComponentName,
		BinaryName:  nginxBinaryName,
		BinaryPath:  filepath.Join(binaryDir, nginxBinaryName),
		ServiceUnit: nginxServiceUnit,
		Service: TemplateConfig{
			Source: nginxServiceTemplate,
		},
		DropIns: map[string]TemplateConfig{
			nginxOverrideName: {
				Source: nginxOverrideTemplate,
			},
		},
	})
}
