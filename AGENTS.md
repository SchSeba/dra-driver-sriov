# AGENTS.md — AI Agent Guide for dra-driver-sriov

This document provides guidance for AI agents working with the
**dra-driver-sriov** codebase. It covers project purpose, architecture, code
layout, conventions, build/test workflows, and common pitfalls.

---

## Project Overview

**dra-driver-sriov** is a Kubernetes
[Dynamic Resource Allocation (DRA)](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
driver for SR-IOV (Single Root I/O Virtualization) network devices. It
discovers SR-IOV Virtual Functions (VFs) on a node, advertises them as
schedulable Kubernetes resources via `ResourceSlice`, and prepares/unprepares
VFs when pods claim them through `ResourceClaim` objects.

The driver runs as a DaemonSet on each node with SR-IOV-capable NICs. It
integrates with the kubelet DRA plugin framework, NRI (Node Resource Interface),
CNI (Container Network Interface), and CDI (Container Device Interface) to
manage the full lifecycle of VF allocation to containers.

**Key Kubernetes concepts used:**

- `ResourceSlice` / `ResourceClaim` — DRA resource advertisement and consumption
- `SriovResourcePolicy` — Custom Resource (CRD) for opt-in device advertisement policies
- `DeviceAttributes` — Custom Resource (CRD) for user-defined device attributes
- `VfConfig` — Per-claim configuration embedded in `ResourceClaim` opaque parameters

---

## Repository Structure

```
.
├── cmd/
│   └── dra-driver-sriov/       # Single binary entry point (main.go)
├── pkg/
│   ├── api/
│   │   ├── sriovdra/v1alpha1/  # CRD types: SriovResourcePolicy, DeviceAttributes
│   │   └── virtualfunction/v1alpha1/  # VfConfig type (claim config parameters)
│   ├── cdi/                    # CDI spec file generation
│   ├── cni/                    # CNI plugin invocation (network setup for VFs)
│   ├── consts/                 # Shared constants (driver name, attributes, paths)
│   ├── controller/             # controller-runtime reconciler for SriovResourcePolicy
│   ├── devicestate/            # Device discovery, state tracking, prepare/unprepare
│   ├── driver/                 # DRA kubelet plugin (PrepareResourceClaims, UnprepareResourceClaims)
│   ├── flags/                  # CLI flags, kube client setup, logging config
│   ├── host/                   # Low-level host operations (sysfs, netlink, VF config)
│   ├── nri/                    # NRI plugin (inject devices into containers at runtime)
│   ├── podmanager/             # Tracks prepared devices per pod/claim
│   └── types/                  # Shared types (Config, Flags)
├── deployments/
│   ├── container/              # Dockerfile + container build Makefile
│   └── helm/dra-driver-sriov/  # Helm chart for deployment
├── demo/                       # Example ResourceClaim/Deployment YAML demos
├── docs/design/                # Design documents (e.g., opt-in-advertisement.md)
├── docker/                     # Dockerfile.devel (development build container)
├── hack/                       # Helper scripts (virtual cluster, release, boilerplate)
├── .github/workflows/          # CI (ci.yaml), Release (release.yaml), Helm chart push
├── Makefile                    # Primary build system
├── common.mk                  # Shared Make variables (Go version, tool versions, module)
├── .golangci.yml               # Linter configuration (golangci-lint v2)
├── go.mod / go.sum             # Go module definition
├── CONTRIBUTING.md             # Contribution guidelines
└── README.md                   # User-facing documentation
```

### Key Package Roles

| Package | Responsibility |
|---------|---------------|
| `cmd/dra-driver-sriov` | CLI entry point using `urfave/cli/v2`. Wires up all components and starts the driver. |
| `pkg/driver` | Implements the DRA kubelet plugin interface (`PrepareResourceClaims`, `UnprepareResourceClaims`). Publishes `ResourceSlice`. |
| `pkg/devicestate` | Discovers VFs from sysfs/PCI, maintains device state, handles prepare/unprepare logic, and tracks advertised vs. allocated devices. |
| `pkg/controller` | `controller-runtime` reconciler that watches `SriovResourcePolicy` CRs to determine which devices to advertise. |
| `pkg/host` | All low-level host interactions: reading sysfs, configuring VF drivers (`vfio-pci`, `mlx5_core`, etc.), netlink operations, RDMA. |
| `pkg/cdi` | Generates CDI specification files so container runtimes can inject VF devices into containers. |
| `pkg/cni` | Invokes CNI plugins (`ADD`/`DEL`/`CHECK`) to configure network namespaces for VFs. |
| `pkg/nri` | NRI plugin that hooks into container creation to apply device and network configuration. |
| `pkg/podmanager` | In-memory + checkpoint-based tracking of which devices are prepared for which pod/claim. |
| `pkg/api/sriovdra/v1alpha1` | CRD types: `SriovResourcePolicy` (device selection policy) and `DeviceAttributes` (custom attributes). |
| `pkg/api/virtualfunction/v1alpha1` | `VfConfig` type: per-claim parameters (driver override, interface name, NetworkAttachmentDefinition). |
| `pkg/consts` | Driver name (`sriovnetwork.k8snetworkplumbingwg.io`), device attribute keys, system paths. |
| `pkg/flags` | CLI flag definitions, Kubernetes client construction, logging setup. |
| `pkg/types` | Shared `Config` and `Flags` structs used across packages. |

---

## Architecture & Data Flow

```
┌──────────────────────────────────────────────────────────────┐
│                        Kubernetes API                        │
│   ResourceSlice  ←→  ResourceClaim  ←→  SriovResourcePolicy │
└────────────┬─────────────────┬────────────────┬──────────────┘
             │                 │                │
     PublishResources   Prepare/Unprepare   Reconcile
             │                 │                │
        ┌────┴────┐      ┌────┴────┐     ┌─────┴──────┐
        │ driver  │      │ driver  │     │ controller │
        │ package │      │ package │     │  package   │
        └────┬────┘      └────┬────┘     └─────┬──────┘
             │                │                │
        ┌────┴────────────────┴────────────────┘
        │           devicestate package
        │    (discovery, state, prepare/unprepare)
        └──┬──────────┬──────────┬──────────┬───┐
           │          │          │          │   │
        ┌──┴──┐  ┌────┴──┐  ┌───┴──┐  ┌───┴┐  │
        │host │  │  cdi  │  │ cni  │  │nri │  │
        │(sys)│  │(specs)│  │(net) │  │    │  │
        └─────┘  └───────┘  └──────┘  └────┘  │
                                          ┌────┴──────┐
                                          │podmanager │
                                          │(tracking) │
                                          └───────────┘
```

1. **Device Discovery**: `devicestate` uses `host` to scan sysfs (`/sys/bus/pci/devices`) for SR-IOV VFs, collecting PCI attributes, link type, RDMA capability, etc.
2. **Policy Matching**: The `controller` watches `SriovResourcePolicy` CRs and tells `devicestate` which devices match policy filters (vendors, PCI addresses, PF names, etc.).
3. **Resource Advertisement**: `driver.PublishResources()` publishes matched devices as a `ResourceSlice` via the kubelet plugin helper.
4. **Claim Preparation**: When a pod's `ResourceClaim` is allocated, `driver.PrepareResourceClaims()` calls `devicestate.PrepareDevicesForClaim()` which uses `host` to bind the VF to the requested driver, `cdi` to generate device specs, and stores state via `podmanager`.
5. **Container Setup**: The `nri` plugin hooks into container creation and uses `cni` to configure the network namespace for the VF.
6. **Unprepare**: On pod termination, `UnprepareResourceClaims()` reverses the preparation: unbinds drivers, cleans up CDI specs, and removes state.

---

## Build System

### Prerequisites

- **Go**: Version specified in `common.mk` (currently `1.25`)
- **Container tool**: Docker or Podman (`CONTAINER_TOOL` env var, defaults to `docker`)
- **golangci-lint v2**: Downloaded automatically by `make lint`

### Key Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile all packages (no binary output) |
| `make cmds` / `make binaries` | Build the `dra-driver-sriov` binary |
| `make test` | Run unit tests with coverage (uses `envtest` for controller tests) |
| `make test-coverage` | Same as `test` but with atomic coverage mode |
| `make coverage` | Run tests and print coverage report (excludes mock files) |
| `make lint` | Run `golangci-lint` on `./cmd/...` and `./pkg/...` |
| `make fmt` | Apply `gofmt -s` to all Go files |
| `make assert-fmt` | Check formatting (fails CI if files aren't formatted) |
| `make vet` | Run `go vet` |
| `make generate` | Run all code generation (deepcopy, CRDs, mocks) |
| `make generate-deepcopy` | Generate `zz_generated.deepcopy.go` files via `controller-gen` |
| `make generate-crds` | Generate CRD YAML manifests into the Helm chart templates |
| `make mock-generate` | Generate mock files via `mockgen` (from `go:generate` directives) |
| `make vendor` | Update Go vendor directory |
| `make all` | Run `check`, `test`, `build` |
| `make check` | Run `assert-fmt`, `vet`, `lint` |

### Container Build

```bash
# Build the container image (from deployments/container/)
cd deployments/container && make centos9

# Or from root (with docker- prefix for containerized builds):
make docker-build
make docker-test
make docker-lint
```

### Docker-Prefixed Targets

Any make target can be prefixed with `docker-` (e.g., `make docker-test`) to
run it inside a development container. This ensures a consistent build
environment matching CI.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GOOS` | `linux` | Target OS |
| `GOARCH` | `amd64` | Target architecture |
| `CONTAINER_TOOL` | `docker` | Container build tool (`docker` or `podman`) |
| `IMAGE_NAME` | `ghcr.io/k8snetworkplumbingwg/dra-driver-sriov` | Container image name |
| `VERSION` | auto-detected from git/branch | Image version tag |

---

## Code Conventions

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) and
  [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- **Formatting**: `gofmt -s` and `goimports` are enforced. The `goimports`
  local prefix is `github.com/k8snetworkplumbingwg/dra-driver-sriov`.
- **Import ordering**: standard library → third-party → local packages
  (separated by blank lines, enforced by `goimports`).

### Linting

The project uses **golangci-lint v2** (`.golangci.yml`). Key enabled linters:

- `errcheck`, `govet`, `staticcheck`, `gosec`, `ineffassign`, `unused`
- `goconst` (min-len: 2, min-occurrences: 2)
- `depguard` (blocks `logrus`; use `klog` instead)
- `misspell` (US locale)

Mock files (`*_mock.go`) and generated files (`zz_generated.*.go`) are excluded
from linting.

### Logging

- Use **`klog/v2`** — never `logrus` or `fmt.Printf` for operational logging.
- Obtain loggers from context: `logger := klog.FromContext(ctx)`.
- Use structured logging with key-value pairs: `logger.Info("msg", "key", value)`.
- Use verbosity levels: `V(1)` for operational details, `V(2)` for retry/debug,
  `V(3)` for verbose data dumps.

### Error Handling

- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Use named error types/sentinels from upstream packages where appropriate
  (e.g., `kubeletplugin.ErrRecoverable`).
- Return errors up the call chain; don't log-and-return (pick one).

### Naming Conventions

- **Branch names**: Use `dev/` prefix for feature branches (e.g., `dev/some-feature`).
- **File names**: Snake case (e.g., `device_state.go`, `dra_hook.go`).
- **Test files**: `*_test.go` in the same package (white-box testing).
- **Test suites**: Each package with tests has a `*_suite_test.go` bootstrapping Ginkgo.
- **Mock files**: Generated in `mock/` subdirectories or as `*_mock.go` files.
- **Generated files**: Prefixed with `zz_generated.` (e.g., `zz_generated.deepcopy.go`).

### License Headers

All Go source files must include the Apache 2.0 license header. Templates are
in `hack/boilerplate.go.txt` (regular files) and
`hack/boilerplate.generatego.txt` (generated files).

---

## Testing

### Framework

- **Ginkgo v2** (`github.com/onsi/ginkgo/v2`) + **Gomega** (`github.com/onsi/gomega`)
  for BDD-style tests.
- Each testable package has a `*_suite_test.go` file that bootstraps the Ginkgo
  test runner:
  ```go
  func TestPkgName(t *testing.T) {
      RegisterFailHandler(Fail)
      RunSpecs(t, "PkgName Suite")
  }
  ```
- Dot-imports of `ginkgo/v2` and `gomega` are allowed (whitelisted in
  `.golangci.yml`).

### Mocking

- Uses **`go.uber.org/mock`** (mockgen) for interface mocking.
- Mocks are generated via `make mock-generate` which runs `go generate ./...`.
- Mock source files live in `mock/` subdirectories under their respective
  packages (e.g., `pkg/host/mock/`, `pkg/cni/mock/`).

### Controller Tests (envtest)

- Controller tests (e.g., `pkg/controller/`) use
  **`sigs.k8s.io/controller-runtime/tools/setup-envtest`** to spin up a local
  API server and etcd.
- `KUBEBUILDER_ASSETS` is automatically set by `make test`.
- The envtest Kubernetes version is configured via `ENVTEST_K8S_VERSION`
  (currently `1.34.x`).

### Running Tests

```bash
# Run all tests
make test

# Run tests with atomic coverage
make test-coverage

# View coverage report (excludes mocks)
make coverage

# Run a specific test
go test -v ./pkg/driver/... -run "TestDriverSuite"
```

---

## Custom Resource Definitions (CRDs)

### SriovResourcePolicy (`pkg/api/sriovdra/v1alpha1`)

Defines **which** SR-IOV VFs to advertise. Contains resource filters (by
vendor, device ID, PCI address, PF name, driver) and optional node selectors.
Only devices matching at least one policy are published in the `ResourceSlice`.

### DeviceAttributes (`pkg/api/sriovdra/v1alpha1`)

Allows users to attach **custom attributes** to devices selected by a policy.
Referenced from `SriovResourcePolicy` via label selectors.

### VfConfig (`pkg/api/virtualfunction/v1alpha1`)

Per-claim configuration embedded in `ResourceClaim` opaque parameters.
Controls driver binding (e.g., `vfio-pci`), interface naming, and
NetworkAttachmentDefinition references.

### Code Generation

CRD types require generated code:

```bash
# Regenerate deepcopy methods AND CRD YAML manifests AND mocks
make generate

# Or individually:
make generate-deepcopy   # zz_generated.deepcopy.go files
make generate-crds       # CRD YAML in deployments/helm/dra-driver-sriov/templates/
make mock-generate       # Mock files from go:generate directives
```

**Always run `make generate && make fmt` after modifying API types.** CI checks
that generated files are up to date.

---

## CI Pipeline

CI is defined in `.github/workflows/ci.yaml` and runs on PRs and pushes to
`main`. It consists of four jobs:

| Job | What it checks |
|-----|---------------|
| `generate-and-format` | `make generate && make fmt` — fails if generated/formatted files differ |
| `lint` | `make lint` — golangci-lint |
| `test` | `make test-coverage` — unit tests + coverage upload to Coveralls |
| `build-container` | Container image build (PR only) |
| `functional-tests` | E2E tests on SR-IOV hardware (runs after all other jobs pass) |

### Before Submitting a PR

```bash
make generate   # Regenerate code
make fmt        # Format code
make check      # assert-fmt + vet + lint
make test       # Run all tests
make build      # Ensure it compiles
```

Or all at once: `make all` (runs `check`, `test`, `build`).

---

## Deployment

### Helm Chart

The Helm chart is located at `deployments/helm/dra-driver-sriov/`. CRD YAML
templates are auto-generated by `make generate-crds`.

### Container Image

- **Registry**: `ghcr.io/k8snetworkplumbingwg/dra-driver-sriov`
- **Base image**: CentOS Stream 9
- **Multi-arch**: Release workflow builds multi-arch images

### Virtual Cluster (Development)

Scripts in `hack/` support deploying a virtual Kubernetes cluster for testing:

```bash
make deploy-virtual-k8s-cluster   # Deploy virtual cluster
make delete-virtual-k8s-cluster   # Tear down
make redeploy-operator-virtual-cluster  # Redeploy driver in virtual cluster
```

---

## Common Pitfalls & Tips

1. **Always run `make generate` after modifying API types** — CI will fail if
   `zz_generated.deepcopy.go` or CRD YAML is stale.

2. **Don't use `logrus`** — it is blocked by `depguard`. Use `klog/v2` with
   context-based loggers.

3. **Mock regeneration** — After changing interfaces in `pkg/host/`,
   `pkg/cni/`, or `pkg/devicestate/`, run `make mock-generate` to update mocks.

4. **Import ordering** — Use three groups separated by blank lines: stdlib,
   external, local. The `goimports` local prefix is
   `github.com/k8snetworkplumbingwg/dra-driver-sriov`.

5. **Host package is sysfs-heavy** — `pkg/host/` reads and writes to
   `/sys/bus/pci/devices`, `/sys/class/infiniband`, etc. Tests in this package
   mock the filesystem interactions; do not assume real hardware paths exist.

6. **envtest for controller tests** — The `pkg/controller/` tests require
   `setup-envtest` binaries. `make test` handles this automatically, but
   running `go test` directly requires setting `KUBEBUILDER_ASSETS`.

7. **Single binary architecture** — The entire driver is a single binary
   (`dra-driver-sriov`) that runs all components (DRA plugin, NRI plugin,
   controller, device state manager) in one process.

8. **The `vendor/` directory is gitignored** — Dependencies are managed via
   `go.mod`. Run `make vendor` only if you need a local vendor tree.

9. **Pod lifecycle tracking** — `podmanager` uses a checkpoint file
   (`checkpoint.json`) on disk to persist prepared device state across driver
   restarts. Be aware of this when debugging state issues.

10. **Network setup via NRI + CNI** — Device injection happens at container
    creation time through NRI hooks, not through the kubelet device plugin
    interface. CNI plugins are invoked to configure the VF network namespace.

---

## Key Dependencies

| Dependency | Purpose |
|-----------|---------|
| `k8s.io/dynamic-resource-allocation` | DRA kubelet plugin framework |
| `sigs.k8s.io/controller-runtime` | Controller framework for CRD reconciliation |
| `github.com/containerd/nri` | Node Resource Interface plugin SDK |
| `github.com/containernetworking/cni` | CNI plugin invocation |
| `tags.cncf.io/container-device-interface` | CDI spec generation |
| `github.com/jaypipes/ghw` | Hardware discovery (PCI) |
| `github.com/Mellanox/rdmamap` | RDMA device discovery |
| `github.com/vishvananda/netlink` | Linux netlink interface |
| `github.com/onsi/ginkgo/v2` + `gomega` | Test framework |
| `go.uber.org/mock` | Mock generation |
| `github.com/urfave/cli/v2` | CLI framework |
| `k8s.io/klog/v2` | Structured logging |

---

## Quick Reference

```bash
# Build
make cmds                        # Build binary
make docker-cmds                 # Build in container

# Test
make test                        # Run tests
make coverage                    # Tests + coverage report

# Lint & Format
make lint                        # golangci-lint
make fmt                         # gofmt + goimports
make check                       # All checks (fmt + vet + lint)

# Generate
make generate                    # All code generation
make mock-generate               # Mocks only

# Full pre-PR validation
make all                         # check + test + build
```
