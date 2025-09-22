/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package cni provides integration between ResourceClaims and the (CNI) specification.
// It implements the logic required to attach and detach network interfaces
// for Pods based on ResourceClaims.
package cni

import (
	"context"
	"fmt"
	"os"

	"github.com/SchSeba/dra-driver-sriov/pkg/types"
	"github.com/containerd/nri/pkg/api"
	"github.com/containernetworking/cni/libcni"
	netattdefclientutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/klog/v2"
)

// Runtime represents a CNI (Container Network Interface) runtime environment
// that manages the lifecycle of network attachments for Pods via ResourceClaims.
type Runtime struct {
	CNIConfig  libcni.CNI
	DriverName string
}

// New creates and returns a new CNI Runtime instance.
func New(
	driverName string,
	cniPath []string,
) *Runtime {
	exec := &RawExec{
		Stderr: os.Stderr,
		// ChrootDir: chrootDir,
	}

	rntm := &Runtime{
		CNIConfig:  libcni.NewCNIConfig(cniPath, exec),
		DriverName: driverName,
	}

	return rntm
}

// AttachNetworks attaches network interfaces to a pod based on the provided ResourceClaim.
// It processes the ResourceClaim's device allocation status, extracts CNI configuration for each device,
// and invokes the CNI ADD operation for each relevant device. The results of the CNI operations are used
// to update the ResourceClaim's status with allocated device information.
// If a request fails, an error is returned together with the previous successful device status up to date.
// If the status of a device is already set, CNI ADD will be skipped and the existing status will be preserved.
func (rntm *Runtime) AttachNetwork(ctx context.Context, pod *api.PodSandbox, podNetworkNamespace string, deviceConfig *types.PreparedDevice) (*resourcev1.NetworkDeviceData, error) {
	rt := &libcni.RuntimeConf{
		ContainerID: pod.Id,
		NetNS:       podNetworkNamespace,
		IfName:      deviceConfig.IfName,
		Args: [][2]string{
			{"IgnoreUnknown", "true"},
			{"K8S_POD_NAMESPACE", pod.Namespace},
			{"K8S_POD_NAME", pod.Name},
			{"K8S_POD_INFRA_CONTAINER_ID", pod.Id},
			{"K8S_POD_UID", pod.Uid},
		},
	}
	rawNetConf, err := netattdefclientutils.GetCNIConfigFromSpec(deviceConfig.NetAttachDefConfig, rntm.DriverName)
	if err != nil {
		return nil, fmt.Errorf("failed to GetCNIConfigFromSpec: %v", err)
	}

	confList, err := libcni.ConfFromBytes([]byte(rawNetConf))
	if err != nil {
		return nil, fmt.Errorf("failed to ConfListFromBytes: %v", err)
	}
	klog.FromContext(ctx).V(3).Info("Runtime.AttachNetwork", "deviceConfig", deviceConfig)

	cniResult, err := rntm.CNIConfig.AddNetwork(ctx, confList, rt)
	if err != nil {
		return nil, fmt.Errorf("failed to AddNetwork: %v", err)
	}
	if cniResult == nil {
		return nil, fmt.Errorf("cni result is nil")
	}

	klog.FromContext(ctx).V(3).Info("Runtime.AttachedNetwork", "cniResult", cniResult)
	return cniResultToNetworkData(cniResult)
}

// DetachNetworks detaches all network interfaces associated with a given pod.
// It is typically called during pod teardown to clean up network resources.
func (rntm *Runtime) DetachNetwork(
	ctx context.Context,
	pod *api.PodSandbox,
	podNetworkNamespace string,
	deviceConfig *types.PreparedDevice,
) error {
	klog.FromContext(ctx).Info("Runtime.DetachNetwork", "deviceConfig", deviceConfig)
	rt := &libcni.RuntimeConf{
		ContainerID: pod.Id,
		NetNS:       podNetworkNamespace,
		IfName:      deviceConfig.IfName,
		Args: [][2]string{
			{"IgnoreUnknown", "true"},
			{"K8S_POD_NAMESPACE", pod.Namespace},
			{"K8S_POD_NAME", pod.Name},
			{"K8S_POD_INFRA_CONTAINER_ID", pod.Id},
			{"K8S_POD_UID", pod.Uid},
		},
	}
	rawNetConf, err := netattdefclientutils.GetCNIConfigFromSpec(deviceConfig.NetAttachDefConfig, rntm.DriverName)
	if err != nil {
		return fmt.Errorf("failed to GetCNIConfigFromSpec: %v", err)
	}

	confList, err := libcni.ConfFromBytes(rawNetConf)
	if err != nil {
		return fmt.Errorf("failed to ConfListFromBytes: %v", err)
	}
	klog.FromContext(ctx).V(3).Info("Runtime.DetachNetwork", "deviceConfig", deviceConfig)
	err = rntm.CNIConfig.DelNetwork(ctx, confList, rt)
	if err != nil {
		return fmt.Errorf("failed to DelNetwork: %v", err)
	}

	return nil
}
