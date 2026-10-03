package framework

import (
	"context"
	"strings"

	. "github.com/onsi/gomega"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/dynamic-resource-allocation/deviceattribute"
)

const (
	LabelMultus           = "Multus"
	LabelStandalone       = "Standalone"
	LabelVfio             = "Vfio"
	LabelExtendedResource = "ExtendedResource"
	LabelAlignment        = "Alignment"
	LabelResourcePolicy   = "ResourcePolicy"

	SriovCapableNodeLabelKey   = "feature.node.kubernetes.io/network-sriov.capable"
	SriovCapableNodeLabelValue = "true"

	AlignmentGPUDriverName = "gpu.example.com"
)

// SkipUnlessMultus skips when Multus is not installed or the driver is not in MULTUS mode.
func (c *Clients) SkipUnlessMultus(ctx context.Context) {
	if !c.hasMultus(ctx) {
		skipTestf("Multus not detected in cluster")
	}
	if mode := c.detectDriverMode(ctx); mode != "" && mode != "MULTUS" {
		skipTestf("driver configurationMode=%s (need MULTUS)", mode)
	}
}

// SkipUnlessStandalone skips demos that rely on the NRI attach path (ifName /
// netAttachDefName) when the driver is in MULTUS mode. In MULTUS mode NRI is
// disabled and secondary interfaces only appear via Multus pod annotations.
func (c *Clients) SkipUnlessStandalone(ctx context.Context) {
	if mode := c.detectDriverMode(ctx); mode == "MULTUS" {
		skipTestf("driver configurationMode=MULTUS (need STANDALONE / NRI)")
	}
}

// SkipUnlessAlignment skips when the cluster has no gpu.example.com ResourceSlice with
// deviceattribute.StandardDeviceAttributePCIeRoot. Call SkipUnlessStandalone or SkipUnlessMultus when the
// fixture depends on driver mode.
func (c *Clients) SkipUnlessAlignment(ctx context.Context) {
	ok, err := c.hasResourceSliceDeviceAttribute(ctx, AlignmentGPUDriverName, deviceattribute.StandardDeviceAttributePCIeRoot)
	Expect(err).NotTo(HaveOccurred())
	if !ok {
		skipTestf("no %s ResourceSlice device with %s (run make install-fake-gpu-driver)",
			AlignmentGPUDriverName, deviceattribute.StandardDeviceAttributePCIeRoot)
	}
}

// SkipUnlessSriovCapableNode skips when no node has the SR-IOV capable label.
func (c *Clients) SkipUnlessSriovCapableNode(ctx context.Context) {
	nodes, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: SriovCapableNodeLabelKey + "=" + SriovCapableNodeLabelValue,
	})
	Expect(err).NotTo(HaveOccurred())
	if len(nodes.Items) == 0 {
		skipTestf("no nodes with label %s=%s", SriovCapableNodeLabelKey, SriovCapableNodeLabelValue)
	}
}

// SkipIfNodeMissing skips when the node is absent.
func (c *Clients) SkipIfNodeMissing(ctx context.Context, wantNode string) {
	nodes, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	for _, n := range nodes.Items {
		if n.Name == wantNode {
			return
		}
	}
	skipTestf("node %s not found", wantNode)
}

func (c *Clients) hasMultus(ctx context.Context) bool {
	nss := []string{"kube-system", "multus", "default"}
	for _, ns := range nss {
		pods, err := c.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "name=multus",
		})
		if err == nil && len(pods.Items) > 0 {
			return true
		}
		dss, err := c.Clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, ds := range dss.Items {
			if strings.Contains(strings.ToLower(ds.Name), "multus") {
				return true
			}
		}
	}
	return false
}

func (c *Clients) detectDriverMode(ctx context.Context) string {
	pods, err := c.Clientset.CoreV1().Pods(DriverNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: DriverLabel,
	})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}
	for _, p := range pods.Items {
		for _, ctn := range p.Spec.Containers {
			joined := strings.ToUpper(strings.Join(ctn.Args, " "))
			if strings.Contains(joined, "MULTUS") {
				return "MULTUS"
			}
			if strings.Contains(joined, "STANDALONE") {
				return "STANDALONE"
			}
			for _, env := range ctn.Env {
				if strings.EqualFold(env.Name, "CONFIGURATION_MODE") || strings.EqualFold(env.Name, "DRA_DRIVER_MODE") {
					return strings.ToUpper(env.Value)
				}
			}
		}
	}
	return "STANDALONE"
}

// hasResourceSliceDeviceAttribute reports whether any device on a ResourceSlice for driver
// has a non-empty string value for attrKey.
func (c *Clients) hasResourceSliceDeviceAttribute(ctx context.Context, driver string, attrKey resourceapi.QualifiedName) (bool, error) {
	list, err := c.Clientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, slice := range list.Items {
		if slice.Spec.Driver != driver {
			continue
		}
		for _, device := range slice.Spec.Devices {
			if _, ok := deviceAttributeString(device.Attributes, attrKey); ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// deviceAttributeString returns the string value of a ResourceSlice device attribute, if set and non-empty.
func deviceAttributeString(attrs map[resourceapi.QualifiedName]resourceapi.DeviceAttribute, key resourceapi.QualifiedName) (string, bool) {
	attr, ok := attrs[key]
	if !ok || attr.StringValue == nil || *attr.StringValue == "" {
		return "", false
	}
	return *attr.StringValue, true
}
