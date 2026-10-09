//go:build e2e

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/dra-driver-sriov/test/e2e/framework"
)

var _ = Describe("demo/multus-integration-resource-alignment", Label(framework.LabelMultus, framework.LabelAlignment), Serial, Ordered, func() {
	const (
		ns               = "vf-test10"
		podName          = "pod0"
		container        = "ctr0"
		podClaim         = "vf"
		vfDeviceRequest  = "vf"
		gpuDeviceRequest = "gpu"
	)

	AfterEach(func() {
		clients.Cleanup(ctx, framework.CleanupSpec{
			Namespaces: []string{ns},
			DeviceAttributes: []string{
				"alignment-multus-attrs",
			},
			SriovResourcePolicies: []string{
				"all-devices",
				"alignment-multus-policy",
			},
		})
	})

	It("schedules a Multus pod with PCIe root alignment constraints", func() {
		clients.SkipUnlessMultus(ctx)
		clients.SkipUnlessAlignment(ctx)

		path, err := framework.DemoPath("multus-integration-resource-alignment", "multus-integration-resource-alignment.yaml")
		Expect(err).NotTo(HaveOccurred())

		By("applying fixture")
		_, err = clients.ApplyYAML(ctx, path)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for pod Ready")
		pod := clients.WaitForPodReady(ctx, ns, podName)

		claimName := framework.ResourceClaimNameForPodClaim(pod, podClaim)
		Expect(claimName).NotTo(BeEmpty(), "pod %s has no ResourceClaim for %s", podName, podClaim)

		By("checking VF and GPU share pcieRoot via ResourceSlice attributes")
		clients.ExpectResourceClaimRequestsSharePCIeRoot(ctx, ns, claimName, vfDeviceRequest, gpuDeviceRequest)

		By("checking secondary network interface from Multus")
		clients.ExpectPodLinkInterfacesExact(ctx, ns, podName, container, "lo", "eth0", "net1")
	})
})
