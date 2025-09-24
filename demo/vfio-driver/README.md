# VFIO Driver Configuration Demo

This demo illustrates how to configure SR-IOV Virtual Functions with VFIO-PCI driver for userspace applications like DPDK.

## Overview

This scenario demonstrates:
- Configuring Virtual Functions to use VFIO-PCI driver instead of kernel networking drivers
- Setting up device passthrough for high-performance userspace networking applications
- Enabling vhost-user socket mounting for container networking acceleration

## Components

### 1. DeviceClass Configuration
The `DeviceClass` resource configures how devices should be allocated:
- **selectors**: Uses CEL to filter devices by the SR-IOV DRA driver
- **config**: Specifies VFIO-PCI driver configuration
- **VfConfig parameters**:
  - `driver: vfio-pci`: Binds VF to VFIO-PCI driver for userspace access
  - `addVhostMount: true`: Mounts vhost-user sockets into the container

### 2. Networking Setup
- Creates dedicated namespace (`vf-test2`)
- NetworkAttachmentDefinition with SR-IOV CNI configuration
- Standard networking setup for management interface

### 3. Resource Allocation
- ResourceClaimTemplate requests VF from the VFIO-configured DeviceClass
- Specifies VFIO-PCI driver binding and vhost mounting requirements
- Network attachment for management or control plane connectivity

### 4. Pod Deployment
- Deploys toolbox container for testing and development
- Claims VFIO-configured resource for high-performance networking
- Maintains network admin capabilities for testing

## Use Cases

This configuration is ideal for:
- **DPDK Applications**: High-performance packet processing in userspace
- **NFV Workloads**: Network Function Virtualization with hardware acceleration
- **Data Plane Applications**: Applications requiring direct hardware access
- **High-throughput Networking**: Bypassing kernel network stack for performance

## VFIO vs Kernel Driver

| Aspect | VFIO-PCI | Kernel Driver |
|--------|----------|---------------|
| **Performance** | Higher (userspace) | Lower (kernel overhead) |
| **CPU Usage** | Lower (poll mode) | Higher (interrupt driven) |
| **Latency** | Lower | Higher |
| **Application Type** | Userspace (DPDK) | Kernel networking |
| **Resource Requirements** | More memory | Standard |

## Usage

1. Deploy the VFIO configuration:
   ```bash
   kubectl apply -f vfio-driver-config.yaml
   ```

2. The DRA driver will:
   - Bind the allocated VF to vfio-pci driver
   - Create device nodes in `/dev/vfio/`
   - Mount vhost-user sockets if enabled

3. Applications in the pod can access the VF through:
   - VFIO device files (`/dev/vfio/vfio`, `/dev/vfio/<group>`)
   - DPDK libraries and frameworks
   - Direct userspace networking APIs

## Prerequisites

- VFIO kernel modules loaded (`vfio-pci`, `vfio_iommu_type1`)
- IOMMU enabled in BIOS/UEFI
- Hugepages configured (typically required for DPDK)
- Container runtime with device passthrough support
- Appropriate security policies for device access

## Security Considerations

- VFIO provides secure device isolation through IOMMU
- Containers get direct hardware access - ensure trust boundaries
- Consider using device plugins or security contexts to limit access
