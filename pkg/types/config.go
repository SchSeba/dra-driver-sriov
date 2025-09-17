package types

import (
	"path/filepath"

	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
	"github.com/SchSeba/dra-driver-sriov/pkg/flags"
)

type Flags struct {
	KubeClientConfig flags.KubeClientConfig
	LoggingConfig    *flags.LoggingConfig

	NodeName                      string
	CdiRoot                       string
	KubeletRegistrarDirectoryPath string
	KubeletPluginsDirectoryPath   string
	HealthcheckPort               int
}

type Config struct {
	Flags         *Flags
	K8sClient     flags.ClientSets
	CancelMainCtx func(error)
}

func (c Config) DriverPluginPath() string {
	return filepath.Join(c.Flags.KubeletPluginsDirectoryPath, consts.DriverName)
}
