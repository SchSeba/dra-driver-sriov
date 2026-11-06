/*
Copyright 2026 The Kubernetes Authors.

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

package types

import (
	"context"
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// IsPermanentStatusUpdateError reports whether a failed status update will keep
// failing the same way, so retrying the same request only burns the backoff and
// hides the real error. An invalid (for example a duplicate device key), forbidden
// or not-found response is permanent. Everything else, including conflicts (which
// the caller handles by refetching and merging) and transient failures such as
// server timeouts, throttling and network errors, is worth retrying, so callers
// treat a non-permanent error as retryable rather than enumerate every one.
func IsPermanentStatusUpdateError(err error) bool {
	return apierrors.IsInvalid(err) ||
		apierrors.IsForbidden(err) ||
		apierrors.IsNotFound(err)
}

// ResourceClaimStatusClient is the subset of the generated ResourceClaim client
// that UpdateClaimStatusWithRetry needs. Both the typed client returned by
// ResourceV1().ResourceClaims(namespace) and the fake client used in tests
// satisfy it.
type ResourceClaimStatusClient interface {
	UpdateStatus(ctx context.Context, resourceClaim *resourceapi.ResourceClaim, opts metav1.UpdateOptions) (*resourceapi.ResourceClaim, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourceapi.ResourceClaim, error)
}

// UpdateClaimStatusWithRetry writes claim.Status, retrying on conflict. Each retry
// reapplies this driver's device statuses over the refetched list, so an entry
// another driver wrote during the conflict window survives.
//
// It stops rather than burn the backoff on an error that cannot succeed unchanged,
// or on a claim recreated under the same name, and returns the last API error
// rather than the backoff timeout. Both pkg/driver and pkg/nri use it.
func UpdateClaimStatusWithRetry(
	ctx context.Context,
	claims ResourceClaimStatusClient,
	claim *resourceapi.ResourceClaim,
	driverName string,
	backoff wait.Backoff,
) error {
	logger := klog.FromContext(ctx).WithName("UpdateClaimStatusWithRetry")

	// Snapshot of this driver's desired device statuses, reapplied on top of the
	// latest claim on every conflict.
	desired := claim.Status.Devices

	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		_, updateErr := claims.UpdateStatus(ctx, claim, metav1.UpdateOptions{})
		if updateErr == nil {
			return true, nil
		}

		if apierrors.IsConflict(updateErr) {
			lastErr = updateErr
			logger.V(2).Info("Conflict detected, refreshing claim", "claim", claim.UID)

			freshClaim, fetchErr := claims.Get(ctx, claim.Name, metav1.GetOptions{})
			if fetchErr != nil {
				lastErr = fetchErr
				if IsPermanentStatusUpdateError(fetchErr) {
					return false, fetchErr
				}
				logger.V(2).Info("Failed to fetch fresh claim, retrying", "claim", claim.UID, "error", fetchErr.Error())
				return false, nil
			}

			// A claim deleted and recreated under the same name is a different object,
			// and this driver's status belongs to the one that is gone.
			if freshClaim.UID != claim.UID {
				lastErr = fmt.Errorf("claim %s was replaced (UID %s is now %s): %w",
					claim.Name, claim.UID, freshClaim.UID,
					apierrors.NewNotFound(resourceapi.Resource("resourceclaims"), claim.Name))
				return false, lastErr
			}

			freshClaim.Status.Devices = MergeDeviceStatuses(freshClaim.Status.Devices, desired, driverName)
			claim = freshClaim
			logger.V(2).Info("Refreshed claim, retrying status update", "claim", claim.UID)
			return false, nil
		}

		lastErr = updateErr
		if IsPermanentStatusUpdateError(updateErr) {
			return false, updateErr
		}
		logger.V(2).Info("Retrying claim status update", "claim", claim.UID, "error", updateErr.Error())
		return false, nil
	})

	// ExponentialBackoff reports its own timeout once the steps are exhausted;
	// surface the last real API error instead so the caller can see what failed.
	// A context cancellation is itself the reason the loop ended (for example the
	// NRI runner shutting down), so leave it in place rather than masking it.
	if err != nil && lastErr != nil && ctx.Err() == nil {
		return lastErr
	}
	return err
}
