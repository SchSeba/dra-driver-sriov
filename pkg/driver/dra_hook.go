package driver

import (
	"context"
	"errors"
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	k8stypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
)

func (d *Driver) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[k8stypes.UID]kubeletplugin.PrepareResult, error) {
	result := make(map[k8stypes.UID]kubeletplugin.PrepareResult)
	if len(claims) == 0 {
		return result, nil
	}
	logger := klog.FromContext(ctx).WithName("PrepareResourceClaims")
	logger.V(1).Info("number of claims", "len", len(claims))
	logger.V(3).Info("claims", "claims", claims)

	// lets prepare the claims
	for _, claim := range claims {
		logger.V(3).Info("Preparing claim", "claim", claim.UID)
		result[claim.UID] = d.prepareResourceClaim(ctx, claim)
		logger.V(3).Info("Prepared claim", "claim", claim.UID, "result", result[claim.UID])
	}

	logger.V(3).Info("Prepared claims", "result", result)
	return result, nil
}

func (d *Driver) prepareResourceClaim(ctx context.Context, claim *resourceapi.ResourceClaim) kubeletplugin.PrepareResult {
	logger := klog.FromContext(ctx).WithName("prepareResourceClaim")

	// Get pod info from claim
	if len(claim.Status.ReservedFor) == 0 {
		logger.Error(fmt.Errorf("no pod info found for claim %s/%s/%s", claim.Namespace, claim.Name, claim.UID), "Error preparing devices for claim")
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("no pod info found for claim %s/%s/%s", claim.Namespace, claim.Name, claim.UID),
		}
	} else if len(claim.Status.ReservedFor) > 1 {
		logger.Error(fmt.Errorf("multiple pods found for claim %s/%s/%s not supported", claim.Namespace, claim.Name, claim.UID), "Error preparing devices for claim")
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("multiple pods found for claim %s/%s/%s not supported", claim.Namespace, claim.Name, claim.UID),
		}
	}

	// get the pod UID
	podUID := claim.Status.ReservedFor[0].UID

	// check if the pod claim already is prepared
	preparedDevices, isAlreadyPrepared := d.podManager.Get(podUID, claim.UID)
	if isAlreadyPrepared {
		var prepared []kubeletplugin.Device
		for _, preparedDevice := range preparedDevices {
			prepared = append(prepared, kubeletplugin.Device{
				Requests:     preparedDevice.Device.GetRequestNames(),
				PoolName:     preparedDevice.Device.GetPoolName(),
				DeviceName:   preparedDevice.Device.GetDeviceName(),
				CDIDeviceIDs: preparedDevice.Device.GetCDIDeviceIDs(),
			})
		}
		return kubeletplugin.PrepareResult{Devices: prepared}
	}

	// if the pod claim is not prepared, prepare the devices for the claim
	preparedDevices, err := d.deviceStateManager.PrepareDevices(ctx, claim)
	if err != nil {
		logger.Error(err, "Error preparing devices for claim", "claim", claim.UID)
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("error preparing devices for claim %v: %w", claim.UID, err),
		}
	}
	var prepared []kubeletplugin.Device
	for _, preparedPB := range preparedDevices {
		prepared = append(prepared, kubeletplugin.Device{
			Requests:     preparedPB.Device.GetRequestNames(),
			PoolName:     preparedPB.Device.GetPoolName(),
			DeviceName:   preparedPB.Device.GetDeviceName(),
			CDIDeviceIDs: preparedPB.Device.GetCDIDeviceIDs(),
		})
	}

	err = d.podManager.Set(podUID, claim.UID, preparedDevices)
	if err != nil {
		logger.Error(err, "Error setting prepared devices for pod into pod manager", "pod", podUID)
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("error setting prepared devices for pod %s into pod manager: %w", podUID, err),
		}
	}

	logger.V(3).Info("Returning prepared devices for claim", "claim", claim.UID, "prepared", prepared)
	return kubeletplugin.PrepareResult{Devices: prepared}
}

func (d *Driver) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	logger := klog.FromContext(ctx).WithName("UnprepareResourceClaims")
	logger.V(1).Info("UnprepareResourceClaims is called", "number of claims", len(claims))
	logger.V(3).Info("claims", "claims", claims)
	result := make(map[types.UID]error)

	for _, claim := range claims {
		result[claim.UID] = d.unprepareResourceClaim(ctx, claim)
	}

	logger.V(3).Info("Unprepared claims", "result", result)
	return result, nil
}

func (d *Driver) unprepareResourceClaim(ctx context.Context, claim kubeletplugin.NamespacedObject) error {
	logger := klog.FromContext(ctx).WithName("unprepareResourceClaim")
	logger.V(1).Info("Unpreparing resource claim", "claim", claim.UID)
	logger.V(3).Info("claim", "claim", claim)

	preparedDevices, found := d.podManager.GetByClaim(claim)
	if !found {
		return nil
	}

	if err := d.deviceStateManager.Unprepare(string(claim.UID), preparedDevices); err != nil {
		return fmt.Errorf("error unpreparing devices for claim %v: %w", claim.UID, err)
	}

	// delete the claim from the pod manager
	err := d.podManager.DeleteClaim(claim)
	if err != nil {
		logger.Error(err, "Error deleting claim from pod manager", "claim", claim.UID)
		return fmt.Errorf("error deleting claim %s from pod manager: %w", claim.UID, err)
	}
	return nil
}

func (d *Driver) HandleError(ctx context.Context, err error, msg string) {
	utilruntime.HandleErrorWithContext(ctx, err, msg)
	if !errors.Is(err, kubeletplugin.ErrRecoverable) && d.cancelCtx != nil {
		d.cancelCtx(fmt.Errorf("fatal background error: %w", err))
	}
}
