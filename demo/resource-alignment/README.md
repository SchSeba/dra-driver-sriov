# Resource Alignment Demo (STANDALONE)

This demo schedules a pod that requests an SR-IOV VF and a fake GPU with a
cross-driver `matchAttribute` on `resource.kubernetes.io/pcieRoot`. The VF is
attached through the driver **NRI** path (`VfConfig`: `ifName`, `netAttachDefName`).

For the same alignment constraint with **Multus** attach, see
[`../multus-integration-resource-alignment/`](../multus-integration-resource-alignment/).

## Prerequisite: deploy DRA SR-IOV driver in STANDALONE mode

This fixture uses opaque `VfConfig` on the ResourceClaim. The driver must run with
`kubeletPlugin.configurationMode=STANDALONE` (NRI enabled).

From the repository root on a virtual cluster:

```bash
make deploy-single-node-virtual-cluster-standalone
```

Or on an existing cluster:

```bash
export KUBECONFIG="${KUBECONFIG:-$HOME/.kcli/clusters/dra/auth/kubeconfig}"
make install-fake-gpu-driver
```

## Prerequisite: gpu.example.com fake GPU driver

Alignment needs a second DRA publisher that also exposes `resource.kubernetes.io/pcieRoot`
on ResourceSlice devices. This repo installs
[kubernetes-sigs/dra-example-driver](https://github.com/kubernetes-sigs/dra-example-driver)
via [`hack/install-dra-example-gpu-driver.sh`](../../hack/install-dra-example-gpu-driver.sh)
(pinned release **v0.5.0** image and Helm chart from `registry.k8s.io`, mirrored into the cluster registry).

Virtual-cluster deploy installs the fake GPU driver automatically (`GPU_PUBLISH_PCIE_ROOT=true`,
`PCIE_ROOTS` discovered from SR-IOV ResourceSlices). On an existing cluster, use
`make install-fake-gpu-driver`.

- Override roots manually if needed: `PCIE_ROOTS=pci0000:14,pci0000:28 make install-fake-gpu-driver`.
- To build from upstream git instead (e.g. `main`): `DRA_EXAMPLE_DRIVER_BUILD=1 DRA_EXAMPLE_DRIVER_REF=main make install-fake-gpu-driver`.

Fake GPUs assign `pcieRoot` in round-robin order from `PCIE_ROOTS`; values must
overlap what the SR-IOV driver publishes so `matchAttribute` can succeed.

## What this manifest creates

The `resource-alignment.yaml` file creates:

1. **`SriovResourcePolicy`** (`all-devices`) in namespace `dra-driver-sriov`
   - Advertises SR-IOV devices with an empty config (all devices on the node)
2. **Namespace** `vf-test5`
3. **NetworkAttachmentDefinition** `vf-test1`
   - SR-IOV CNI config with IPAM subnet `10.0.1.0/24`
4. **`ResourceClaimTemplate`** `resource-alignment`
   - Requests one VF (`sriovnetwork.k8snetworkplumbingwg.io`) and one GPU (`gpu.example.com`)
   - **`matchAttribute`**: `resource.kubernetes.io/pcieRoot`
   - **`VfConfig`**: `ifName: net1`, `netAttachDefName: vf-test1`
5. **Pod** `pod0`
   - Consumes the claim via `resourceClaimTemplateName: resource-alignment`

## Deploy

```bash
kubectl apply -f resource-alignment.yaml
```

## Verify

1. Confirm both drivers publish overlapping `pcieRoot` values:

   ```bash
   kubectl get resourceslices -o yaml | grep -E 'gpu.example.com|pcieRoot|sriovnetwork'
   ```

2. Check namespace resources and claim allocation:

   ```bash
   kubectl get sriovresourcepolicy,net-attach-def,resourceclaimtemplate,resourceclaim,pod -n vf-test5
   ```

3. Wait for the pod and inspect interfaces:

   ```bash
   kubectl wait --for=condition=Ready pod/pod0 -n vf-test5 --timeout=5m
   kubectl exec -n vf-test5 pod0 -- ip link show
   kubectl exec -n vf-test5 pod0 -- ip addr show net1
   ```

## Run e2e

```bash
export KUBECONFIG="${KUBECONFIG:-$HOME/.kcli/clusters/dra/auth/kubeconfig}"
make e2e-workloads E2E_LABEL_FILTER='Standalone && Alignment'
```

## Troubleshooting

- **Alignment claim not allocated**
  - Confirm `gpu.example.com` devices have `resource.kubernetes.io/pcieRoot` and values
    overlap SR-IOV VF attributes (`PCIE_ROOTS` / `make install-fake-gpu-driver`).
- **Pod pending**
  - Check `ResourceClaim` events; ensure `all-devices` policy is applied and VFs exist.
- **No `net1` in pod**
  - Driver must be STANDALONE; verify `VfConfig` and NAD namespace match the claim.

## Cleanup

```bash
kubectl delete -f resource-alignment.yaml
```

## Related demos

- `../multus-integration-resource-alignment` — same `matchAttribute`, Multus attach
- `../single-vf-claim` — STANDALONE VF without GPU alignment
