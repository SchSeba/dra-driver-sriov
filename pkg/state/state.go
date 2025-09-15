package state

import (
	"fmt"
	"slices"
	"sync"

	configapi "github.com/SchSeba/dra-driver-sriov/pkg/api/virtualfunction/v1alpha1"
	"github.com/SchSeba/dra-driver-sriov/pkg/cdi"
	"github.com/SchSeba/dra-driver-sriov/pkg/checkpoint"
	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
	"github.com/SchSeba/dra-driver-sriov/pkg/types"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

type DeviceState struct {
	sync.Mutex
	cdi               *cdi.CDIHandler
	allocatable       types.AllocatableDevices
	checkpointManager checkpointmanager.CheckpointManager
}

func NewDeviceState(config *types.Config) (*DeviceState, error) {
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

	checkpointManager, err := checkpointmanager.NewCheckpointManager(config.DriverPluginPath())
	if err != nil {
		return nil, fmt.Errorf("unable to create checkpoint manager: %v", err)
	}

	state := &DeviceState{
		cdi:               cdi,
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
	}

	checkpoints, err := state.checkpointManager.ListCheckpoints()
	if err != nil {
		return nil, fmt.Errorf("unable to list checkpoints: %v", err)
	}

	for _, c := range checkpoints {
		if c == consts.DriverPluginCheckpointFile {
			klog.Infof("Found checkpoint: %s", c)
			return state, nil
		}
	}

	checkpoint := checkpoint.NewCheckpoint()
	if err := state.checkpointManager.CreateCheckpoint(consts.DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}
	klog.Infof("Created checkpoint: %v", *checkpoint)

	klog.Infof("Created State: %v", state)
	return state, nil
}

func (s *DeviceState) GetAllocatableDevices() types.AllocatableDevices {
	return s.allocatable
}

func (s *DeviceState) Prepare(claim *resourceapi.ResourceClaim) ([]*drapbv1.Device, error) {
	s.Lock()
	defer s.Unlock()

	claimUID := string(claim.UID)
	checkpoint := checkpoint.NewCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(consts.DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] != nil {
		return preparedClaims[claimUID].GetDevices(), nil
	}

	preparedDevices, err := s.prepareDevices(claim)
	if err != nil {
		return nil, fmt.Errorf("prepare failed: %v", err)
	}

	if err = s.cdi.CreateClaimSpecFile(claimUID, preparedDevices); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	preparedClaims[claimUID] = preparedDevices
	if err := s.checkpointManager.CreateCheckpoint(consts.DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return preparedClaims[claimUID].GetDevices(), nil
}

func (s *DeviceState) prepareDevices(claim *resourceapi.ResourceClaim) (types.PreparedDevices, error) {
	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim not yet allocated")
	}

	// Retrieve the full set of device configs for the driver.
	configs, err := GetOpaqueDeviceConfigs(
		configapi.Decoder,
		consts.DriverName,
		claim.Status.Allocation.Devices.Config,
	)
	if err != nil {
		return nil, fmt.Errorf("error getting opaque device configs: %v", err)
	}

	// Add the default GPU Config to the front of the config list with the
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
			return nil, fmt.Errorf("requested GPU is not allocatable: %v", result.Device)
		}
		for _, c := range slices.Backward(configs) {
			if len(c.Requests) == 0 || slices.Contains(c.Requests, result.Request) {
				configResultsMap[c.Config] = append(configResultsMap[c.Config], &result)
				break
			}
		}
	}

	// Normalize, validate, and apply all configs associated with devices that
	// need to be prepared. Track container edits generated from applying the
	// config to the set of device allocation results.
	perDeviceCDIContainerEdits := make(types.PerDeviceCDIContainerEdits)
	for c, results := range configResultsMap {
		// Cast the opaque config to a GpuConfig
		var config *configapi.VfConfig
		switch castConfig := c.(type) {
		case *configapi.VfConfig:
			config = castConfig
		default:
			return nil, fmt.Errorf("runtime object is not a regognized configuration")
		}

		// Normalize the config to set any implied defaults.
		if err := config.Normalize(); err != nil {
			return nil, fmt.Errorf("error normalizing Vf config: %w", err)
		}

		// Validate the config to ensure its integrity.
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("error validating Vf config: %w", err)
		}

		// Apply the config to the list of results associated with it.
		containerEdits, err := s.applyConfig(config, results)
		if err != nil {
			return nil, fmt.Errorf("error applying Vf config: %w", err)
		}

		// Merge any new container edits with the overall per device map.
		for k, v := range containerEdits {
			perDeviceCDIContainerEdits[k] = v
		}
	}

	// Walk through each config and its associated device allocation results
	// and construct the list of prepared devices to return.
	var preparedDevices types.PreparedDevices
	for _, results := range configResultsMap {
		for _, result := range results {
			device := &types.PreparedDevice{
				Device: drapbv1.Device{
					RequestNames: []string{result.Request},
					PoolName:     result.Pool,
					DeviceName:   result.Device,
					CDIDeviceIDs: s.cdi.GetClaimDevices(string(claim.UID), []string{result.Device}),
				},
				ContainerEdits: perDeviceCDIContainerEdits[result.Device],
			}
			preparedDevices = append(preparedDevices, device)
		}
	}

	return preparedDevices, nil
}

// applyConfig applies a configuration to a set of device allocation results.
//
// In this example driver there is no actual configuration applied. We simply
// define a set of environment variables to be injected into the containers
// that include a given device. A real driver would likely need to do some sort
// of hardware configuration as well, based on the config passed in.
func (s *DeviceState) applyConfig(config *configapi.VfConfig, results []*resourceapi.DeviceRequestAllocationResult) (types.PerDeviceCDIContainerEdits, error) {
	perDeviceEdits := make(types.PerDeviceCDIContainerEdits)
	for _, result := range results {
		deviceInfo, exist := s.allocatable[result.Device]
		if !exist {
			return nil, fmt.Errorf("device %s not found in allocatable devices", result.Device)
		}
		envs := []string{
			fmt.Sprintf("VF_DEVICE_%s=%s", result.Device, *deviceInfo.Attributes["pciAddress"].StringValue),
		}

		edits := &cdispec.ContainerEdits{
			Env: envs,
		}

		perDeviceEdits[result.Device] = &cdiapi.ContainerEdits{ContainerEdits: edits}
	}

	return perDeviceEdits, nil
}

func (s *DeviceState) Unprepare(claimUID string) error {
	s.Lock()
	defer s.Unlock()

	checkpoint := checkpoint.NewCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(consts.DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] == nil {
		return nil
	}

	if err := s.unprepareDevices(claimUID, preparedClaims[claimUID]); err != nil {
		return fmt.Errorf("unprepare failed: %v", err)
	}

	err := s.cdi.DeleteClaimSpecFile(claimUID)
	if err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %v", err)
	}

	delete(preparedClaims, claimUID)
	if err := s.checkpointManager.CreateCheckpoint(consts.DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return nil
}

func (s *DeviceState) unprepareDevices(claimUID string, devices types.PreparedDevices) error {
	return nil
}
