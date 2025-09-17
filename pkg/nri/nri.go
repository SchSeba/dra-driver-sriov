package nri

import (
	"context"
	"fmt"

	"github.com/SchSeba/dra-driver-sriov/pkg/cni"
	"github.com/SchSeba/dra-driver-sriov/pkg/podmanager"
	"github.com/SchSeba/dra-driver-sriov/pkg/types"
	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// Plugin Represents a NRI plugin catching RunPodSandbox and StopPodSandbox events to
// call CNI ADD/DEL based on ResourceClaim attached to pods.
type Plugin struct {
	stub       stub.Stub
	podManager *podmanager.PodManager
	cniRuntime *cni.Runtime
	// PodResourceStore PodResourceStore
	// UpdateStatusFunc UpdateStatus
}

func NewNRIPlugin(config *types.Config, podManager *podmanager.PodManager, cniRuntime *cni.Runtime) (*Plugin, error) {
	p := &Plugin{
		podManager: podManager,
		cniRuntime: cniRuntime,
	}
	var err error
	p.stub, err = stub.New(p)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin stub: %w", err)
	}

	return p, nil
}

func (p *Plugin) Start(ctx context.Context) error {
	logger := klog.FromContext(ctx).WithName("NRI Start")
	logger.Info("Starting NRI plugin")
	err := p.stub.Start(ctx)
	if err != nil {
		logger.Error(err, "Failed to start NRI plugin")
		return fmt.Errorf("failed to start NRI plugin: %w", err)
	}
	return nil
}

func (p *Plugin) Stop() {
	p.stub.Stop()
}

func (p *Plugin) RunPodSandbox(ctx context.Context, pod *api.PodSandbox) error {
	logger := klog.FromContext(ctx).WithName("NRI RunPodSandbox")
	logger.Info("RunPodSandbox", "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace)

	devices, found := p.podManager.GetDevicesByPodUID(k8stypes.UID(pod.Uid))
	if !found {
		logger.Info("No prepared devices found for pod", "pod.UID", pod.Uid)
		return nil
	}

	// if we don't have a network namespace, we can't attach networks
	// so we skip the network attachment
	networkNamespace := getNetworkNamespace(pod)
	if networkNamespace == "" {
		logger.Info("No network namespace found for pod skipping network attachment", "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace)
		return nil
	}

	for _, device := range devices {
		device.PodNetworkNamespace = networkNamespace
		device.PodSandboxID = pod.Id
		logger.Info("Attaching network", "device", device)

		networkDeviceData, err := p.cniRuntime.AttachNetwork(ctx, device)
		if err != nil {
			logger.Error(err, "Failed to attach network", "deviceName", device.Device.DeviceName, "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace)
			return fmt.Errorf("failed to attach network: %w", err)
		}

		logger.Info("Attached network", "deviceName", device.Device.DeviceName, "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace, "networkDeviceData", networkDeviceData)
		// TODO: CONTINUE HERE
		// IMPLEMENT THE AllocatedDeviceStatus HERE WITH THE networkDeviceData and Data
	}

	return nil
}

func (p *Plugin) StopPodSandbox(ctx context.Context, pod *api.PodSandbox) error {
	logger := klog.FromContext(ctx).WithName("NRI StopPodSandbox")
	logger.Info("StopPodSandbox", "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace)

	devices, found := p.podManager.GetDevicesByPodUID(k8stypes.UID(pod.Uid))
	if !found {
		logger.Info("No prepared devices found for pod", "pod.UID", pod.Uid)
		return nil
	}

	networkNamespace := getNetworkNamespace(pod)
	if networkNamespace == "" {
		return fmt.Errorf("error getting network namespace for pod '%s' in namespace '%s'", pod.Name, pod.Namespace)
	}

	for _, device := range devices {
		device.PodNetworkNamespace = networkNamespace
		device.PodSandboxID = pod.Id
		logger.Info("Detaching network", "device", device)

		err := p.cniRuntime.DetachNetwork(ctx, device)
		if err != nil {
			logger.Error(err, "Failed to detach network", "deviceName", device.Device.DeviceName, "pod.UID", pod.Uid, "pod.Name", pod.Name, "pod.Namespace", pod.Namespace)
			return fmt.Errorf("error CNI.DetachNetwork for pod '%s' (uid: %s) in namespace '%s': %v", pod.Name, pod.Uid, pod.Namespace, err)
		}
	}
	return nil
}
