package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	cli "github.com/urfave/cli/v2"

	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"

	"github.com/SchSeba/dra-driver-sriov/pkg/cni"
	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
	"github.com/SchSeba/dra-driver-sriov/pkg/devicestate"
	"github.com/SchSeba/dra-driver-sriov/pkg/driver"
	"github.com/SchSeba/dra-driver-sriov/pkg/flags"
	"github.com/SchSeba/dra-driver-sriov/pkg/nri"
	"github.com/SchSeba/dra-driver-sriov/pkg/podmanager"
	"github.com/SchSeba/dra-driver-sriov/pkg/types"
)

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	flagsOptions := &types.Flags{
		LoggingConfig: flags.NewLoggingConfig(),
	}
	cliFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "node-name",
			Usage:       "The name of the node to be worked on.",
			Required:    true,
			Destination: &flagsOptions.NodeName,
			EnvVars:     []string{"NODE_NAME"},
		},
		&cli.StringFlag{
			Name:        "cdi-root",
			Usage:       "Absolute path to the directory where CDI files will be generated.",
			Value:       "/var/run/cdi",
			Destination: &flagsOptions.CdiRoot,
			EnvVars:     []string{"CDI_ROOT"},
		},
		&cli.StringFlag{
			Name:        "kubelet-registrar-directory-path",
			Usage:       "Absolute path to the directory where kubelet stores plugin registrations.",
			Value:       kubeletplugin.KubeletRegistryDir,
			Destination: &flagsOptions.KubeletRegistrarDirectoryPath,
			EnvVars:     []string{"KUBELET_REGISTRAR_DIRECTORY_PATH"},
		},
		&cli.StringFlag{
			Name:        "kubelet-plugins-directory-path",
			Usage:       "Absolute path to the directory where kubelet stores plugin data.",
			Value:       kubeletplugin.KubeletPluginsDir,
			Destination: &flagsOptions.KubeletPluginsDirectoryPath,
			EnvVars:     []string{"KUBELET_PLUGINS_DIRECTORY_PATH"},
		},
		&cli.IntFlag{
			Name:        "healthcheck-port",
			Usage:       "Port to start a gRPC healthcheck service. When positive, a literal port number. When zero, a random port is allocated. When negative, the healthcheck service is disabled.",
			Value:       -1,
			Destination: &flagsOptions.HealthcheckPort,
			EnvVars:     []string{"HEALTHCHECK_PORT"},
		},
	}
	cliFlags = append(cliFlags, flagsOptions.KubeClientConfig.Flags()...)
	cliFlags = append(cliFlags, flagsOptions.LoggingConfig.Flags()...)

	app := &cli.App{
		Name:            "dra-driver-sriov",
		Usage:           "dra-driver-sriov implements a DRA driver plugin for SR-IOV virtual functions.",
		ArgsUsage:       " ",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				return fmt.Errorf("arguments not supported: %v", c.Args().Slice())
			}
			return flagsOptions.LoggingConfig.Apply()
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			clientSets, err := flagsOptions.KubeClientConfig.NewClientSets()
			if err != nil {
				return fmt.Errorf("create client: %v", err)
			}

			config := &types.Config{
				Flags:     flagsOptions,
				K8sClient: clientSets,
			}

			return RunPlugin(ctx, config)
		},
	}

	return app
}

func RunPlugin(ctx context.Context, config *types.Config) error {
	logger := klog.FromContext(ctx)

	err := os.MkdirAll(config.DriverPluginPath(), 0750)
	if err != nil {
		return err
	}

	info, err := os.Stat(config.Flags.CdiRoot)
	switch {
	case err != nil && os.IsNotExist(err):
		err := os.MkdirAll(config.Flags.CdiRoot, 0750)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("path for cdi file generation is not a directory: '%v'", err)
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	ctx, cancel := context.WithCancelCause(ctx)
	config.CancelMainCtx = cancel

	// create device state manager
	deviceStateManager, err := devicestate.NewDeviceStateManager(config)
	if err != nil {
		return err
	}

	// create pod manager
	podManager, err := podmanager.NewPodManager(config)
	if err != nil {
		return err
	}

	// start driver
	dvr, err := driver.Start(ctx, config, deviceStateManager, podManager)
	if err != nil {
		return fmt.Errorf("failed to start DRA driver: %w", err)
	}

	// create cni runtime
	cniRuntime := cni.New(consts.DriverName, []string{"/opt/cni/bin"})

	// register to NRI
	nriPlugin, err := nri.NewNRIPlugin(config, podManager, cniRuntime)
	if err != nil {
		return fmt.Errorf("failed to create NRI plugin: %w", err)
	}
	err = nriPlugin.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start NRI plugin: %w", err)
	}

	<-ctx.Done()
	// restore default signal behavior as soon as possible in case graceful
	// shutdown gets stuck.
	stop()
	if err := context.Cause(ctx); err != nil && !errors.Is(err, context.Canceled) {
		// A canceled context is the normal case here when the process receives
		// a signal. Only log the error for more interesting cases.
		logger.Error(err, "error from context")
	}
	logger.V(1).Info("Shutting down")
	nriPlugin.Stop()
	err = dvr.Shutdown(logger)
	if err != nil {
		logger.Error(err, "Unable to cleanly shutdown driver")
	}
	logger.V(1).Info("Successful driver shutdown")

	return nil
}
