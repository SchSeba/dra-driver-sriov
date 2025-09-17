package devicestate

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	configapi "github.com/SchSeba/dra-driver-sriov/pkg/api/virtualfunction/v1alpha1"
	"github.com/SchSeba/dra-driver-sriov/pkg/cdi"
	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
	"github.com/SchSeba/dra-driver-sriov/pkg/flags"
	"github.com/SchSeba/dra-driver-sriov/pkg/types"
	drasriovtypes "github.com/SchSeba/dra-driver-sriov/pkg/types"
	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

type DeviceStateManager struct {
	sync.Mutex
	k8sClient   flags.ClientSets
	cdi         *cdi.CDIHandler
	allocatable drasriovtypes.AllocatableDevices
}

func NewDeviceStateManager(config *drasriovtypes.Config) (*DeviceStateManager, error) {
	allocatable, err := DiscoverSriovDevices()
	if err != nil {
		return nil, fmt.Errorf("error enumerating all possible devices: %v", err)
	}

	cdi, err := cdi.NewCDIHandler(config.Flags.CdiRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI handler: %v", err)
	}

	err = cdi.CreateCommonSpecFile()
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for common edits: %v", err)
	}

	state := &DeviceStateManager{
		k8sClient:   config.K8sClient,
		cdi:         cdi,
		allocatable: allocatable,
	}

	return state, nil
}

// GetAllocatableDevices returns the allocatable devices
func (s *DeviceStateManager) GetAllocatableDevices() drasriovtypes.AllocatableDevices {
	return s.allocatable
}

// PrepareDevices prepares the devices for a given claim
// It will return the prepared devices for the claim
func (s *DeviceStateManager) PrepareDevices(ctx context.Context, claim *resourceapi.ResourceClaim) (drasriovtypes.PreparedDevices, error) {
	s.Lock()
	defer s.Unlock()
	logger := klog.FromContext(ctx).WithName("PrepareDevices")

	logger.Info("Preparing devices for claim", "claim", *claim)
	logger.V(3).Info("Claim", "claim", claim)
	preparedDevices, err := s.prepareDevices(ctx, claim)
	if err != nil {
		logger.Error(err, "Prepare failed", "claim", *claim)
		return nil, fmt.Errorf("prepare failed: %v", err)
	}
	if len(preparedDevices) == 0 {
		logger.Error(fmt.Errorf("no prepared devices found for claim"), "Prepare failed", "claim", *claim)
		return nil, fmt.Errorf("no prepared devices found for claim")
	}

	if err = s.cdi.CreateClaimSpecFile(preparedDevices); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	return preparedDevices, nil
}

func (s *DeviceStateManager) prepareDevices(ctx context.Context, claim *resourceapi.ResourceClaim) (drasriovtypes.PreparedDevices, error) {
	logger := klog.FromContext(ctx).WithName("prepareDevices")
	if claim.Status.Allocation == nil {
		logger.Error(fmt.Errorf("claim not yet allocated"), "Prepare failed", "claim", claim.UID)
		return nil, fmt.Errorf("claim not yet allocated")
	}

	// Retrieve the full set of device configs for the driver.
	configs, err := getOpaqueDeviceConfigs(
		configapi.Decoder,
		consts.DriverName,
		claim.Status.Allocation.Devices.Config,
	)
	if err != nil {
		logger.Error(fmt.Errorf("error getting opaque device configs: %v", err), "Prepare failed", "claim", claim.UID)
		return nil, fmt.Errorf("error getting opaque device configs: %v", err)
	}
	logger.V(3).Info("Opaque device configs", "configs", configs)

	configResultsMap, err := s.getConfigResultsMap(configs, claim)
	if err != nil {
		logger.Error(fmt.Errorf("error getting config results map: %v", err), "Prepare failed", "claim", claim.UID)
		return nil, fmt.Errorf("error getting config results map: %v", err)
	}

	// Normalize, validate, and apply all configs associated with devices that
	// need to be prepared. Track container edits generated from applying the
	// config to the set of device allocation results.
	perDeviceCDIContainerEdits := make(drasriovtypes.PerDeviceCDIContainerEdits)
	perDeviceNetAttachDefs := make(drasriovtypes.PerDeviceNetAttachDefs)
	perDeviceIfName := make(drasriovtypes.PerDeviceIfName)
	for c, results := range configResultsMap {
		// Cast the opaque config to a VfConfig
		var config *configapi.VfConfig
		switch castConfig := c.(type) {
		case *configapi.VfConfig:
			config = castConfig
		default:
			return nil, fmt.Errorf("runtime object is not a regognized configuration")
		}
		logger.V(3).Info("Config and results", "config", config, "results", results)

		// Normalize the config to set any implied defaults.
		if err := config.Normalize(); err != nil {
			return nil, fmt.Errorf("error normalizing Vf config: %w", err)
		}

		// Validate the config to ensure its integrity.
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("error validating Vf config: %w", err)
		}

		// Apply the config to the list of results associated with it.
		containerEdits, containerNetAttachDefs, containerIfName, err := s.applyConfig(ctx, claim.GetNamespace(), config, results)
		if err != nil {
			return nil, fmt.Errorf("error applying Vf config: %w", err)
		}

		// Merge any new container edits with the overall per device map.
		for k, v := range containerEdits {
			perDeviceCDIContainerEdits[k] = v
		}

		// Merge any new net attach defs with the overall per device map.
		for k, v := range containerNetAttachDefs {
			perDeviceNetAttachDefs[k] = v
		}
		for k, v := range containerIfName {
			perDeviceIfName[k] = v
		}
	}

	// Walk through each config and its associated device allocation results
	// and construct the list of prepared devices to return.
	preparedDevices := drasriovtypes.PreparedDevices{}
	for _, results := range configResultsMap {
		for _, result := range results {
			device := &drasriovtypes.PreparedDevice{
				ClaimNamespacedName: kubeletplugin.NamespacedObject{
					NamespacedName: k8stypes.NamespacedName{
						Name:      claim.Name,
						Namespace: claim.Namespace,
					},
					UID: claim.UID,
				},
				PodUID:       claim.Status.ReservedFor[0].UID,
				PodName:      claim.Status.ReservedFor[0].Name,
				PodNamespace: claim.Namespace,
				Device: drapbv1.Device{
					RequestNames: []string{result.Request},
					PoolName:     result.Pool,
					DeviceName:   result.Device,
					CDIDeviceIDs: s.cdi.GetClaimDevices(string(claim.UID), []string{result.Device}),
				},
				ContainerEdits:     perDeviceCDIContainerEdits[result.Device],
				NetAttachDefConfig: perDeviceNetAttachDefs[result.Device],
				IfName:             perDeviceIfName[result.Device],
			}
			preparedDevices = append(preparedDevices, device)
		}
	}

	logger.V(3).Info("Prepared devices", "preparedDevices", preparedDevices)
	return preparedDevices, nil
}

// applyConfig applies a configuration to a set of device allocation results.
//
// In this example driver there is no actual configuration applied. We simply
// define a set of environment variables to be injected into the containers
// that include a given device. A real driver would likely need to do some sort
// of hardware configuration as well, based on the config passed in.
func (s *DeviceStateManager) applyConfig(
	ctx context.Context,
	namespace string,
	vfConfig *configapi.VfConfig,
	deviceRequestAllocationResults []*resourceapi.DeviceRequestAllocationResult) (
	drasriovtypes.PerDeviceCDIContainerEdits,
	drasriovtypes.PerDeviceNetAttachDefs,
	drasriovtypes.PerDeviceIfName,
	error) {

	logger := klog.FromContext(ctx).WithName("applyConfig")
	logger.V(3).Info("Applying config to device allocation results", "vfConfig", *vfConfig, "deviceRequestAllocationResults", deviceRequestAllocationResults)
	perDeviceEdits := make(drasriovtypes.PerDeviceCDIContainerEdits)
	perDeviceNetAttachDefs := make(drasriovtypes.PerDeviceNetAttachDefs)
	perDeviceIfName := make(drasriovtypes.PerDeviceIfName)

	for _, deviceRequestAllocation := range deviceRequestAllocationResults {
		deviceInfo, exist := s.allocatable[deviceRequestAllocation.Device]
		if !exist {
			logger.Error(fmt.Errorf("device %s not found in allocatable devices", deviceRequestAllocation.Device), "Apply config failed", "vfConfig", vfConfig, "deviceRequestAllocationResults", deviceRequestAllocationResults)
			return nil, nil, nil, fmt.Errorf("device %s not found in allocatable devices", deviceRequestAllocation.Device)
		}
		// per device environment variables
		envs := []string{
			fmt.Sprintf("VF_DEVICE_%s=%s", strings.ReplaceAll(deviceRequestAllocation.Device, "-", "_"), *deviceInfo.Attributes[consts.AttributePciAddress].StringValue),
			fmt.Sprintf("NET_ATTACH_DEF_NAME=%s", vfConfig.NetAttachDefName),
		}

		edits := &cdispec.ContainerEdits{
			Env: envs,
		}

		perDeviceEdits[deviceRequestAllocation.Device] = &cdiapi.ContainerEdits{ContainerEdits: edits}

		// Get the net attach def information
		netAttachDef := &netattdefv1.NetworkAttachmentDefinition{}
		err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      vfConfig.NetAttachDefName,
			Namespace: namespace,
		}, netAttachDef)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error getting net attach def for net attach def %s: %w", vfConfig.NetAttachDefName, err)
		}

		// Convert to sriov-cni compatible netconf with deviceID (PCI address)
		pciAddress := *deviceInfo.Attributes[consts.AttributePciAddress].StringValue
		netConf, err := drasriovtypes.AddDeviceIDToNetConf(netAttachDef.Spec.Config, pciAddress)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error converting net attach def config to sriov-cni format: %w", err)
		}
		perDeviceNetAttachDefs[deviceRequestAllocation.Device] = netConf

		perDeviceIfName[deviceRequestAllocation.Device] = vfConfig.IfName
	}

	return perDeviceEdits, perDeviceNetAttachDefs, perDeviceIfName, nil
}

func (s *DeviceStateManager) Unprepare(claimUID string, preparedDevices drasriovtypes.PreparedDevices) error {
	s.Lock()
	defer s.Unlock()

	if err := s.unprepareDevices(preparedDevices); err != nil {
		return fmt.Errorf("unprepare failed: %v", err)
	}

	err := s.cdi.DeleteClaimSpecFile(claimUID)
	if err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %v", err)
	}

	return nil
}

// TODO: Implement this
func (s *DeviceStateManager) unprepareDevices(_ drasriovtypes.PreparedDevices) error {
	return nil
}

func (s *DeviceStateManager) getConfigResultsMap(configs []*types.OpaqueDeviceConfig, claim *resourceapi.ResourceClaim) (map[runtime.Object][]*resourceapi.DeviceRequestAllocationResult, error) {
	// Add the default Config to the front of the config list with the
	// lowest precedence. This guarantees there will be at least one config in
	// the list with len(Requests) == 0 for the lookup below.
	configs = slices.Insert(configs, 0, &types.OpaqueDeviceConfig{
		Requests: []string{},
		Config:   configapi.DefaultVfConfig(),
	})

	// Look through the configs and figure out which one will be applied to
	// each device allocation result based on their order of precedence.
	configResultsMap := make(map[runtime.Object][]*resourceapi.DeviceRequestAllocationResult)
	for _, result := range claim.Status.Allocation.Devices.Results {
		if _, exists := s.allocatable[result.Device]; !exists {
			return nil, fmt.Errorf("requested VF is not allocatable: %v", result.Device)
		}
		for _, c := range slices.Backward(configs) {
			if len(c.Requests) == 0 || slices.Contains(c.Requests, result.Request) {
				configResultsMap[c.Config] = append(configResultsMap[c.Config], &result)
				break
			}
		}
	}
	return configResultsMap, nil
}
