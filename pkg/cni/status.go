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

package cni

import (
	"fmt"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	resourcev1 "k8s.io/api/resource/v1"
)

func cniResultToNetworkData(result cnitypes.Result) (*resourcev1.NetworkDeviceData, error) {
	networkData := &resourcev1.NetworkDeviceData{}

	cniResult, err := cni100.NewResultFromResult(result)
	if err != nil {
		return nil, fmt.Errorf("failed to NewResultFromResult result (%v): %v", result, err)
	}

	for _, ip := range cniResult.IPs {
		networkData.IPs = append(networkData.IPs, ip.Address.String())
	}

	for _, ifs := range cniResult.Interfaces {
		// Only pod interfaces can have sandbox information
		if ifs.Sandbox != "" {
			networkData.InterfaceName = ifs.Name
			networkData.HardwareAddress = ifs.Mac
		}
	}

	return networkData, nil
}
