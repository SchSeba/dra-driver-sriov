package devicestate

import (
	"fmt"
	"strconv"
	"strings"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
	"github.com/SchSeba/dra-driver-sriov/pkg/types"
)

var (
	helpers HelpersInterface
)

func init() {
	helpers = NewHelpers()
}

type PFInfo struct {
	PciAddress  string
	NetName     string
	VendorID    string
	DeviceID    string
	Address     string
	EswitchMode string
}

func DiscoverSriovDevices() (types.AllocatableDevices, error) {
	logger := klog.LoggerWithName(klog.Background(), "DiscoverSriovDevices")
	pfList := []PFInfo{}
	resourceList := types.AllocatableDevices{}

	logger.Info("Starting SR-IOV device discovery")

	pci, err := helpers.PCI()
	if err != nil {
		logger.Error(err, "Failed to get PCI info")
		return nil, fmt.Errorf("error getting PCI info: %v", err)
	}

	devices := pci.Devices
	if len(devices) == 0 {
		logger.Info("No PCI devices found")
		return nil, fmt.Errorf("could not retrieve PCI devices")
	}

	logger.Info("Found PCI devices", "count", len(devices))

	for _, device := range devices {
		logger.V(2).Info("Processing PCI device", "address", device.Address, "class", device.Class.ID)

		devClass, err := strconv.ParseInt(device.Class.ID, 16, 64)
		if err != nil {
			logger.Error(err, "Unable to parse device class, skipping device",
				"address", device.Address, "class", device.Class.ID)
			continue
		}
		if devClass != NetClass {
			logger.V(3).Info("Skipping non-network device", "address", device.Address, "class", devClass)
			continue
		}

		// TODO: exclude devices used by host system
		if helpers.IsSriovVF(device.Address) {
			logger.V(2).Info("Skipping VF device", "address", device.Address)
			continue
		}

		pfNetName := helpers.TryGetInterfaceName(device.Address)
		if pfNetName == "" {
			logger.Error(nil, "Unable to get interface name for device, skipping", "address", device.Address)
			continue
		}

		eswitchMode := helpers.GetNicSriovMode(device.Address)
		logger.Info("Found SR-IOV PF device",
			"address", device.Address,
			"interface", pfNetName,
			"vendor", device.Vendor.ID,
			"device", device.Product.ID,
			"eswitchMode", eswitchMode)

		pfList = append(pfList, PFInfo{
			PciAddress:  device.Address,
			NetName:     pfNetName,
			VendorID:    device.Vendor.ID,
			DeviceID:    device.Product.ID,
			Address:     device.Address,
			EswitchMode: eswitchMode,
		})
	}

	logger.Info("Processing SR-IOV PF devices", "pfCount", len(pfList))

	for _, pfInfo := range pfList {
		logger.V(1).Info("Getting VF list for PF", "pf", pfInfo.NetName, "address", pfInfo.Address)

		vfList, err := helpers.GetVFList(pfInfo.Address)
		if err != nil {
			logger.Error(err, "Failed to get VF list for PF", "pf", pfInfo.NetName, "address", pfInfo.Address)
			return nil, fmt.Errorf("error getting VF list: %v", err)
		}

		logger.Info("Found VFs for PF", "pf", pfInfo.NetName, "vfCount", len(vfList))

		for _, vfPciAddress := range vfList {
			deviceName := strings.ReplaceAll(vfPciAddress, ":", "-")
			deviceName = strings.ReplaceAll(deviceName, ".", "-")

			logger.V(2).Info("Adding VF device to resource list",
				"deviceName", deviceName,
				"vfAddress", vfPciAddress,
				"pf", pfInfo.NetName)

			resourceList[deviceName] = resourceapi.Device{
				Name: deviceName,
				Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
					consts.AttributeVendorID: {
						StringValue: ptr.To(pfInfo.VendorID),
					},
					consts.AttributeDeviceID: {
						StringValue: ptr.To(pfInfo.DeviceID),
					},
					consts.AttributePciAddress: {
						StringValue: ptr.To(vfPciAddress),
					},
					consts.AttributePFName: {
						StringValue: ptr.To(pfInfo.NetName),
					},
					consts.AttributeEswitchMode: {
						StringValue: ptr.To(pfInfo.EswitchMode),
					},
				},
			}
		}
	}

	logger.Info("SR-IOV device discovery completed", "totalDevices", len(resourceList))
	return resourceList, nil
}
