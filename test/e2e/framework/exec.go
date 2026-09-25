package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod runs a command in a pod container and returns combined stdout+stderr.
func (c *Clients) ExecInPod(ctx context.Context, namespace, pod, container string, command ...string) (string, error) {
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if err != nil {
		return out, fmt.Errorf("exec %v in %s/%s: %w (stderr: %s)", command, namespace, pod, err, errOut)
	}
	if errOut != "" && out == "" {
		return errOut, nil
	}
	if errOut != "" {
		return out + "\n" + errOut, nil
	}
	return out, nil
}

type ipLinkJSON struct {
	IfName string `json:"ifname"`
}

// linkInterfaceNames parses stdout from `ip -json link show` (a JSON array of link objects).
func linkInterfaceNames(out string) ([]string, error) {
	var entries []ipLinkJSON
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("unmarshal ip -json link show: %w", err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.IfName
	}
	return names, nil
}

// podHasLinkInterface reports whether want appears in ifnames. want "eth0" also
// matches the pod primary veth peer name eth0@ifindex from `ip -json link show`.
func podHasLinkInterface(ifnames []string, want string) bool {
	for _, name := range ifnames {
		if name == want {
			return true
		}
		if want == "eth0" && strings.HasPrefix(name, "eth0@") {
			return true
		}
	}
	return false
}

func normalizePodLinkIfName(name string) string {
	if strings.HasPrefix(name, "eth0@") {
		return "eth0"
	}
	return name
}

func normalizedPodLinkInterfaceNames(ifnames []string) []string {
	normalized := make([]string, len(ifnames))
	for i, name := range ifnames {
		normalized[i] = normalizePodLinkIfName(name)
	}
	return normalized
}

func (c *Clients) expectPodLinkInterfaces(ctx context.Context, namespace, pod, container string, exact bool, want ...string) {
	out, err := c.ExecInPod(ctx, namespace, pod, container, "ip", "-json", "link", "show")
	Expect(err).NotTo(HaveOccurred(), "ip -json link show in %s/%s", namespace, pod)
	ifnames, err := linkInterfaceNames(out)
	Expect(err).NotTo(HaveOccurred(), "parse link list in %s/%s:\n%s", namespace, pod, out)
	if exact {
		Expect(normalizedPodLinkInterfaceNames(ifnames)).To(ConsistOf(want),
			"pod link interfaces in %s/%s\n%s", namespace, pod, out)
		return
	}
	for _, name := range want {
		Expect(podHasLinkInterface(ifnames, name)).To(BeTrue(),
			"expected interface %q in %v\n%s", name, ifnames, out)
	}
}

// ExpectPodLinkInterfaces runs ip -json link show in the pod and asserts each
// listed interface exists. eth0 also matches veth peer names such as eth0@if5.
func (c *Clients) ExpectPodLinkInterfaces(ctx context.Context, namespace, pod, container string, want ...string) {
	c.expectPodLinkInterfaces(ctx, namespace, pod, container, false, want...)
}

// ExpectPodLinkInterfacesExact is like ExpectPodLinkInterfaces but also rejects
// any interface not listed in want. eth0 still matches veth peer names such as eth0@if5.
func (c *Clients) ExpectPodLinkInterfacesExact(ctx context.Context, namespace, pod, container string, want ...string) {
	c.expectPodLinkInterfaces(ctx, namespace, pod, container, true, want...)
}

// ExpectInterfaceHasAddress asserts ifName exists in the pod and has an IP address.
func (c *Clients) ExpectInterfaceHasAddress(ctx context.Context, namespace, pod, container, ifName string) {
	var out string
	var err error
	out, err = c.ExecInPod(ctx, namespace, pod, container, "ip", "addr", "show", ifName)
	Expect(err).NotTo(HaveOccurred(), "ip addr show %s", ifName)
	Expect(out).To(ContainSubstring(ifName))
	Expect(out).To(Or(ContainSubstring("inet "), ContainSubstring("inet6 ")),
		"interface %s has no address:\n%s", ifName, out)
}

// ExpectPathExists asserts that path exists inside the pod.
func (c *Clients) ExpectPathExists(ctx context.Context, namespace, pod, container, path string) {
	_, err := c.ExecInPod(ctx, namespace, pod, container, "test", "-e", path)
	Expect(err).NotTo(HaveOccurred(), "path %s should exist in %s/%s", path, namespace, pod)
}
