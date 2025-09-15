# DRA Driver for SR-IOV Virtual Functions

A Kubernetes Dynamic Resource Allocation (DRA) driver that enables exposure and management of SR-IOV virtual functions as cluster resources.

## Overview

This project implements a DRA driver that allows Kubernetes workloads to request and use SR-IOV virtual functions through the native Kubernetes resource allocation system. The driver integrates with the kubelet plugin system to manage SR-IOV VF lifecycle, including discovery, allocation, and cleanup.

## Features

- **Dynamic Resource Allocation**: Leverages Kubernetes DRA framework for SR-IOV VF management
- **CDI Integration**: Uses Container Device Interface for device injection into containers
- **Kubernetes Native**: Integrates seamlessly with standard Kubernetes resource request/limit model
- **Health Monitoring**: Built-in health check endpoints for monitoring driver status
- **Helm Deployment**: Easy deployment through Helm charts

## Requirements

- Kubernetes 1.34.0 or later (with DRA support enabled)
- SR-IOV capable network hardware
- Container runtime with CDI support
- Go 1.24.0 or later (for building from source)

## Building

To build the container image, use the following command:

```bash
CONTAINER_TOOL=podman IMAGE_NAME=localhost/dra-driver-sriov VERSION=latest make -f deployments/container/Makefile
```

You can customize the build by setting different environment variables:
- `CONTAINER_TOOL`: Container tool to use (docker, podman)
- `IMAGE_NAME`: Container image name and registry
- `VERSION`: Image tag version

### Building Binaries

To build just the binaries without containerizing:

```bash
make cmds
```

Or to build for specific platforms:

```bash
GOOS=linux GOARCH=amd64 make cmds
```

## Deployment

Deploy the DRA driver using Helm:

```bash
helm upgrade -i sriov-dra --create-namespace -n dra-driver-sriov ./deployments/helm/dra-driver-sriov/
```

### Configuration Options

The Helm chart supports various configuration options through `values.yaml`:

- **Image Configuration**: Customize image repository, tag, and pull policy
- **Resource Limits**: Set resource requests and limits for driver components  
- **Node Selection**: Configure node selectors and tolerations
- **Logging**: Adjust log verbosity and format
- **Security**: Configure security contexts and service accounts

Example custom deployment:

```bash
helm upgrade -i sriov-dra \
  --create-namespace -n dra-sriov-driver \
  --set image.tag=v0.1.0 \
  --set logging.level=5 \
  ./deployments/helm/dra-driver-sriov/
```

## Usage

Once deployed, workloads can request SR-IOV virtual functions using ResourceClaimTemplates:

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: sriov-vf
spec:
  spec:
    devices:
      requests:
      - name: vf
        exactly:
          deviceClassName: virtualfunction.sriovnetwork.openshift.io
```

Then reference the claim in your Pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: sriov-workload
spec:
  containers:
  - name: app
    image: your-app:latest
    resources:
      claims:
      - name: vf
  resourceClaims:
  - name: vf
    resourceClaimTemplateName: sriov-vf
```

### Example Workloads

The `demo/` directory contains several example YAML files demonstrating different usage patterns:

- `gpu-test1.yaml`: Two pods each requesting one distinct GPU
- `gpu-test2.yaml`: More complex multi-container scenarios
- Additional examples showing various resource allocation patterns

## Project Structure

```
├── cmd/
│   └── dra-driver-sriov/          # Main driver executable
├── pkg/
│   ├── driver/                    # Core driver implementation
│   ├── state/                     # Device state management
│   ├── cdi/                       # CDI integration
│   ├── checkpoint/                # State persistence
│   ├── cni/                       # CNI plugin integration
│   ├── types/                     # Type definitions
│   ├── consts/                    # Constants
│   └── flags/                     # Command-line flag handling
├── deployments/
│   ├── container/                 # Container build configuration
│   └── helm/                      # Helm chart
├── demo/                          # Example workload configurations
│   ├── scripts/                   # Demo automation scripts
│   └── *.yaml                     # Example Pod/ResourceClaim manifests
├── hack/                          # Build and development scripts
├── test/                          # Test suites
└── vendor/                        # Go module dependencies
```

### Key Components

- **Driver**: Main gRPC service implementing DRA kubelet plugin interface
- **State Manager**: Tracks available and allocated SR-IOV virtual functions  
- **CDI Generator**: Creates Container Device Interface specifications for VFs
- **Health Check**: Monitors driver health and readiness
- **Checkpoint Manager**: Persists allocation state across restarts

## Development

### Prerequisites

- Go 1.24.0+
- Make
- Container tool (Docker/Podman)
- Kubernetes cluster with DRA enabled

### Building from Source

```bash
# Clone the repository
git clone https://github.com/SchSeba/dra-driver-sriov.git
cd dra-driver-sriov

# Build binaries
make build

# Run tests
make test

# Build container image
make -f deployments/container/Makefile
```

### Testing

The project includes unit tests and end-to-end tests:

```bash
# Run unit tests
make test

# Run with coverage
make coverage

# Run linting and format checks
make check
```

## Contributing

We welcome contributions to the DRA Driver for SR-IOV Virtual Functions project!

### How to Contribute

1. **Fork the repository** on GitHub
2. **Create a feature branch** from `main`
3. **Make your changes** following the coding standards
4. **Add tests** for new functionality
5. **Ensure all tests pass** with `make test check`
6. **Submit a pull request** with a clear description

### Development Guidelines

- Follow Go conventions and use `gofmt` for formatting
- Write unit tests for new code
- Update documentation for user-facing changes
- Use semantic commit messages
- Ensure backward compatibility when possible

### Code Style

- Run `make fmt` to format code
- Run `make check` to verify linting and style
- Follow Kubernetes coding conventions
- Add appropriate logging with structured fields

### Reporting Issues

Please use GitHub Issues to report bugs or request features:

- Use clear, descriptive titles
- Provide detailed reproduction steps for bugs
- Include relevant logs and configuration
- Specify Kubernetes and driver versions

### License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

This project builds upon:
- [Kubernetes Dynamic Resource Allocation](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
- [Container Device Interface](https://github.com/cncf-tags/container-device-interface)
- [SR-IOV CNI](https://github.com/k8snetworkplumbingwg/sriov-cni)
