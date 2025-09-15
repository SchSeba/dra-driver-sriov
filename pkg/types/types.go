package types

import (
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

type VirtualFunction struct {
	DeviceID       string
	VendorID       string
	Driver         string
	NumaNode       int
	PFName         string
	RootPciAddress string
}

type VirtualFunctions []*VirtualFunction

type AllocatableDevices map[string]resourceapi.Device
type PreparedDevices []*PreparedDevice
type PreparedClaims map[string]PreparedDevices
type PerDeviceCDIContainerEdits map[string]*cdiapi.ContainerEdits

type OpaqueDeviceConfig struct {
	Requests []string
	Config   runtime.Object
}

type PreparedDevice struct {
	drapbv1.Device
	ContainerEdits *cdiapi.ContainerEdits
}

func (pds PreparedDevices) GetDevices() []*drapbv1.Device {
	var devices []*drapbv1.Device
	for _, pd := range pds {
		devices = append(devices, &pd.Device)
	}
	return devices
}
