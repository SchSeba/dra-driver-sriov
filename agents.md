# AI Agent Configuration

## Project: DRA Driver SRIOV

This file provides context for AI agents (Claude, Copilot, etc.) when working with the DRA Driver SRIOV project.

## Overview

The DRA (Dynamic Resource Allocation) Driver for SR-IOV is a Kubernetes driver that enables dynamic allocation of SR-IOV (Single Root I/O Virtualization) network resources using the Kubernetes DRA framework.

## Repository Structure

- `cmd/dra-driver-sriov/` - Main entry point for the driver binary
- `pkg/` - Core Go packages
  - `pkg/api/` - API type definitions and CRDs
  - `pkg/cdi/` - CDI (Container Device Interface) support
  - `pkg/cni/` - CNI plugin integration
  - `pkg/consts/` - Shared constants
  - `pkg/controller/` - Kubernetes controller logic
  - `pkg/devicestate/` - Device state tracking
  - `pkg/driver/` - SR-IOV driver implementation
  - `pkg/flags/` - CLI flag definitions
  - `pkg/host/` - Host-level operations
  - `pkg/nri/` - NRI (Node Resource Interface) plugin
  - `pkg/podmanager/` - Pod lifecycle management
  - `pkg/types/` - Shared type definitions
- `deployments/` - Deployment configurations
  - `deployments/container/` - Container build (Dockerfile + Makefile)
  - `deployments/helm/` - Helm chart for Kubernetes deployment
- `hack/` - Helper scripts (cluster deploy, release, codegen boilerplate)

## Development Guidelines

- Written in Go (version defined in `common.mk`)
- Uses Kubernetes client-go and controller-runtime
- Follow existing code patterns and conventions
- Run `make test` before submitting changes
- Run `make lint` to check code style
- Run `make check` to run all checks (assert-fmt, vet, lint)

## Make Targets

### Build

| Target | Description |
|--------|-------------|
| `make build` | Compile all Go packages |
| `make binaries` | Build command binaries (alias for `cmds`) |
| `make cmds` | Build all binaries under `cmd/` |
| `make cmd-<name>` | Build a specific command binary (e.g., `make cmd-dra-driver-sriov`) |
| `make all` | Run checks, tests, and build |

### Code Quality & Checks

| Target | Description |
|--------|-------------|
| `make check` | Run all check targets (`assert-fmt`, `vet`, `lint`) |
| `make lint` | Run `golangci-lint` on `./cmd/...` and `./pkg/...` |
| `make vet` | Run `go vet` on all packages |
| `make fmt` | Apply `gofmt -s` formatting to all Go files |
| `make assert-fmt` | Assert all Go files are properly formatted (fails if not) |
| `make misspell` | Check for common misspellings |

### Testing

| Target | Description |
|--------|-------------|
| `make test` | Run all unit tests with coverage (uses `envtest` for K8s API) |
| `make test-coverage` | Run tests with atomic coverage mode (for CI coverage reporting) |
| `make coverage` | Run tests and display per-function coverage (excludes `_mock.go` files) |
| `make e2e-tests` | Deploy a virtual K8s cluster and run end-to-end tests |

### Code Generation

| Target | Description |
|--------|-------------|
| `make generate` | Run all generators (deepcopy, CRDs, mocks) |
| `make generate-deepcopy` | Generate `zz_generated.deepcopy.go` files for API types |
| `make generate-crds` | Generate CRD manifests into the Helm chart templates |
| `make mock-generate` | Generate mock implementations using `mockgen` |
| `make gomock` | Install the `mockgen` tool |

### Dependencies

| Target | Description |
|--------|-------------|
| `make vendor` | Update the Go vendor directory (`go mod vendor`) |

### Container Images

| Target | Description |
|--------|-------------|
| `make docker-<target>` | Run any make target inside a development container (e.g., `make docker-build`, `make docker-test`) |

For building the production container image, use the `deployments/container/` Makefile:

```bash
cd deployments/container
make centos9         # Build the driver container image
make push            # Push the container image
```

### Helm Chart

| Target | Description |
|--------|-------------|
| `make chart-prepare` | Prepare the Helm chart for release (pass `VERSION=v1.0.0`) |
| `make chart-push` | Push the Helm chart to the registry (pass `VERSION=v1.0.0`) |

### Virtual Cluster (Development)

| Target | Description |
|--------|-------------|
| `make deploy-virtual-k8s-cluster` | Deploy a virtual K8s cluster for development/testing |
| `make delete-virtual-k8s-cluster` | Tear down the virtual K8s cluster |
| `make redeploy-operator-virtual-cluster` | Redeploy the operator on the virtual cluster |

### Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GOOS` | `linux` | Target OS for Go builds |
| `GOARCH` | `amd64` | Target architecture for Go builds |
| `CONTAINER_TOOL` | `docker` | Container runtime (`docker` or `podman`) |
| `IMAGE_NAME` | `ghcr.io/k8snetworkplumbingwg/dra-driver-sriov` | Container image name |
| `VERSION` | auto-detected from git | Image/chart version tag |
| `ENVTEST_K8S_VERSION` | `1.34.x` | Kubernetes version for envtest |
