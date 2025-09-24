package consts_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/SchSeba/dra-driver-sriov/pkg/consts"
)

var _ = Describe("Consts", func() {
	Context("Constants", func() {
		It("should have correct group name", func() {
			Expect(consts.GroupName).To(Equal("sriovnetwork.openshift.io"))
		})

		It("should have correct driver name", func() {
			Expect(consts.DriverName).To(Equal("sriovnetwork.openshift.io"))
		})

		It("should have correct checkpoint file name", func() {
			Expect(consts.DriverPluginCheckpointFile).To(Equal("checkpoint.json"))
		})

		It("should have correct standard attribute prefix", func() {
			Expect(consts.StandardAttributePrefix).To(Equal("resource.kubernetes.io"))
		})

		It("should have correct attributes with driver name prefix", func() {
			expectedAttributes := map[string]string{
				"pciAddress":   consts.DriverName + "/pciAddress",
				"PFName":       consts.DriverName + "/PFName",
				"EswitchMode":  consts.DriverName + "/EswitchMode",
				"vendor":       consts.DriverName + "/vendor",
				"deviceID":     consts.DriverName + "/deviceID",
				"pfDeviceID":   consts.DriverName + "/pfDeviceID",
				"vfID":         consts.DriverName + "/vfID",
				"resourceName": consts.DriverName + "/resourceName",
			}

			Expect(consts.AttributePciAddress).To(Equal(expectedAttributes["pciAddress"]))
			Expect(consts.AttributePFName).To(Equal(expectedAttributes["PFName"]))
			Expect(consts.AttributeEswitchMode).To(Equal(expectedAttributes["EswitchMode"]))
			Expect(consts.AttributeVendorID).To(Equal(expectedAttributes["vendor"]))
			Expect(consts.AttributeDeviceID).To(Equal(expectedAttributes["deviceID"]))
			Expect(consts.AttributePFDeviceID).To(Equal(expectedAttributes["pfDeviceID"]))
			Expect(consts.AttributeVFID).To(Equal(expectedAttributes["vfID"]))
			Expect(consts.AttributeResourceName).To(Equal(expectedAttributes["resourceName"]))
		})

		It("should have correct attributes with standard prefix", func() {
			Expect(consts.AttributeNumaNode).To(Equal(consts.StandardAttributePrefix + "/numaNode"))
			Expect(consts.AttributeParentPciAddress).To(Equal(consts.StandardAttributePrefix + "/pcieRoot"))
		})

		It("should have correct network device constants", func() {
			Expect(consts.NetClass).To(Equal(0x02))
			Expect(consts.SysBusPci).To(Equal("/sys/bus/pci/devices"))
		})
	})

	Context("Backoff configuration", func() {
		It("should have valid backoff configuration", func() {
			backoff := consts.Backoff

			Expect(backoff.Duration).To(Equal(100 * time.Millisecond))
			Expect(backoff.Factor).To(Equal(2.0))
			Expect(backoff.Jitter).To(Equal(0.1))
			Expect(backoff.Steps).To(Equal(5))
			Expect(backoff.Cap).To(Equal(2 * time.Second))
		})

		It("should be a valid wait.Backoff", func() {
			// Test that the backoff configuration is valid by using it
			backoff := consts.Backoff

			// Test some properties that are expected for a proper backoff
			Expect(backoff.Duration).To(BeNumerically(">", 0))
			Expect(backoff.Factor).To(BeNumerically(">", 1.0))
			Expect(backoff.Jitter).To(BeNumerically(">=", 0))
			Expect(backoff.Jitter).To(BeNumerically("<=", 1.0))
			Expect(backoff.Steps).To(BeNumerically(">", 0))
			Expect(backoff.Cap).To(BeNumerically(">", backoff.Duration))
		})

		It("should be compatible with wait.Backoff usage", func() {
			backoff := consts.Backoff

			// Test that we can use it with wait.ExponentialBackoff
			callCount := 0
			err := wait.ExponentialBackoff(backoff, func() (bool, error) {
				callCount++
				if callCount >= 3 {
					return true, nil
				}
				return false, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(3))
		})
	})
})
