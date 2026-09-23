package framework

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/dynamic-resource-allocation/deviceattribute"
)

// ResourceClaimNameForPodClaim returns the generated ResourceClaim name for a pod resource claim ref.
func ResourceClaimNameForPodClaim(pod *corev1.Pod, podClaim string) string {
	for _, st := range pod.Status.ResourceClaimStatuses {
		if st.Name == podClaim && st.ResourceClaimName != nil {
			return *st.ResourceClaimName
		}
	}
	return ""
}

// ExpectResourceClaimRequestsSharePCIeRoot asserts that each named device request in an
// allocated ResourceClaim resolved to devices with the same resource.kubernetes.io/pcieRoot
// on their ResourceSlices (cross-driver matchAttribute alignment).
func (c *Clients) ExpectResourceClaimRequestsSharePCIeRoot(ctx context.Context, namespace, claimName string, requestNames ...string) {
	claim := c.WaitForResourceClaimAllocated(ctx, namespace, claimName)

	want := make(map[string]struct{}, len(requestNames))
	for _, name := range requestNames {
		want[name] = struct{}{}
	}

	allocation := claim.Status.Allocation
	Expect(allocation).NotTo(BeNil(), "claim %s/%s has no status.allocation", namespace, claimName)
	results := allocation.Devices.Results
	Expect(results).NotTo(BeEmpty(), "claim %s/%s has no allocation device results", namespace, claimName)

	for _, res := range results {
		request := res.Request
		if _, ok := want[request]; !ok {
			continue
		}
		Expect(res.Driver).NotTo(BeEmpty(), "allocation result for request %q missing driver", request)
		Expect(res.Pool).NotTo(BeEmpty(), "allocation result for request %q missing pool", request)
		Expect(res.Device).NotTo(BeEmpty(), "allocation result for request %q missing device", request)
	}

	// Retry: pool generation may advance after the claim is allocated; pcieRoot lookup
	// only reads the highest generation, so slices can lag briefly during driver updates.
	Eventually(func(g Gomega) {
		rootsByRequest := make(map[string]string, len(requestNames))
		for _, res := range results {
			request := res.Request
			if _, ok := want[request]; !ok {
				continue
			}
			root, err := c.pcieRootFromResourceSliceDevice(ctx, res.Driver, res.Pool, res.Device)
			g.Expect(err).NotTo(HaveOccurred(), "request %q allocated %s/%s/%s", request, res.Driver, res.Pool, res.Device)
			rootsByRequest[request] = root
		}

		for _, request := range requestNames {
			g.Expect(rootsByRequest).To(HaveKey(request), "no allocation result for request %q in claim %s", request, claimName)
		}

		ref := rootsByRequest[requestNames[0]]
		g.Expect(ref).NotTo(BeEmpty(), "pcieRoot empty for request %q", requestNames[0])
		for _, request := range requestNames[1:] {
			g.Expect(rootsByRequest[request]).To(Equal(ref),
				"expected requests %q and %q to share pcieRoot %q", requestNames[0], request, ref)
		}
	}).WithTimeout(DefaultTimeout).WithPolling(DefaultInterval).Should(Succeed())
}

// pcieRootFromResourceSliceDevice looks up deviceattribute.StandardDeviceAttributePCIeRoot
// for the device identified by driver, pool, and device name on a ResourceSlice.
func (c *Clients) pcieRootFromResourceSliceDevice(ctx context.Context, driver, pool, deviceName string) (string, error) {
	list, err := c.Clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	// During pool updates, old and new ResourceSlices may coexist; use only the
	// current generation for this driver/pool (highest Pool.Generation).
	var generation int64
	for _, slice := range list.Items {
		if slice.Spec.Driver != driver || slice.Spec.Pool.Name != pool {
			continue
		}
		if slice.Spec.Pool.Generation > generation {
			generation = slice.Spec.Pool.Generation
		}
	}

	attrKey := deviceattribute.StandardDeviceAttributePCIeRoot
	for _, slice := range list.Items {
		if slice.Spec.Driver != driver || slice.Spec.Pool.Name != pool {
			continue
		}
		if slice.Spec.Pool.Generation != generation {
			continue
		}
		for _, device := range slice.Spec.Devices {
			if device.Name != deviceName {
				continue
			}
			root, ok := deviceAttributeString(device.Attributes, attrKey)
			if !ok {
				return "", fmt.Errorf("device %s/%s/%s has no %s attribute", driver, pool, deviceName, attrKey)
			}
			return root, nil
		}
	}
	return "", fmt.Errorf("device %s/%s/%s not found on any ResourceSlice", driver, pool, deviceName)
}
