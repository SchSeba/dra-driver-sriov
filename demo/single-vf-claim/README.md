# Single Virtual Function Claim Demo

This demo demonstrates the basic scenario of requesting a single SR-IOV Virtual Function for container networking acceleration.

## Overview

This scenario shows:
- Basic SR-IOV Virtual Function allocation using DRA (Dynamic Resource Allocation)
- Standard kernel-mode networking with SR-IOV acceleration
- Simple container deployment with high-performance networking

## Components

### 1. Networking Setup
- Creates dedicated namespace (`vf-test1`)
- NetworkAttachmentDefinition with SR-IOV CNI plugin configuration
- IPAM setup using host-local plugin with subnet `10.0.1.0/24`
- Standard SR-IOV network settings (VLAN 0, spoofchk on, trust on)

### 2. Single VF Resource Claim
The `ResourceClaimTemplate` configuration:
- **count**: Implicitly 1 (single VF request)
- **deviceClassName**: Uses the default SR-IOV device class
- **VfConfig parameters**:
  - `ifName: net1`: Network interface name in the container
  - `netAttachDefName: vf-test1`: References the NetworkAttachmentDefinition
  - Standard kernel driver (default behavior)

### 3. Pod Deployment
- Simple toolbox container for testing and validation
- Standard networking capabilities (NET_ADMIN, NET_RAW)
- Resource claim binding for the single VF
- Sleep command keeps container running for testing

## Use Cases

Single VF allocation is ideal for:
- **Getting Started**: Learning SR-IOV with DRA
- **Simple Applications**: Applications needing basic SR-IOV acceleration
- **Development/Testing**: Development and testing environments
- **Resource-constrained Environments**: When only one high-performance interface is needed
- **Standard Networking**: Applications using kernel networking stack with hardware acceleration

## Network Interface Behavior

With single VF allocation:
1. **Container Interfaces**:
   - `eth0`: Default pod network (cluster networking)
   - `net1`: SR-IOV interface (high-performance networking)

2. **Traffic Routing**:
   - Cluster traffic uses `eth0`
   - Application traffic can use `net1` for performance
   - Both interfaces can be active simultaneously

## Allocation Process

1. **Resource Request**: Pod requests single VF through resource claim
2. **Device Discovery**: DRA driver finds available VF
3. **Driver Binding**: VF remains bound to kernel driver (default)
4. **Interface Creation**: SR-IOV CNI creates `net1` interface in pod
5. **IP Assignment**: IPAM assigns IP from configured subnet
6. **Pod Ready**: Pod starts with both standard and SR-IOV networking

## Usage

1. Deploy the configuration:
   ```bash
   kubectl apply -f single-vf.yaml
   ```

2. Verify pod deployment:
   ```bash
   kubectl get pods -n vf-test1
   kubectl describe pod pod0 -n vf-test1
   ```

3. Check network configuration:
   ```bash
   # List interfaces
   kubectl exec -n vf-test1 pod0 -- ip link show
   
   # Check IP addresses
   kubectl exec -n vf-test1 pod0 -- ip addr show
   
   # Verify SR-IOV interface
   kubectl exec -n vf-test1 pod0 -- ip addr show net1
   ```

4. Test network connectivity:
   ```bash
   # Test cluster networking (eth0)
   kubectl exec -n vf-test1 pod0 -- ping kubernetes.default.svc.cluster.local
   
   # Test SR-IOV interface (net1) - requires target IP
   kubectl exec -n vf-test1 pod0 -- ping -I net1 <target-ip>
   ```

## Performance Characteristics

- **Throughput**: Higher than standard pod networking
- **Latency**: Lower latency compared to virtual interfaces
- **CPU Usage**: Efficient packet processing with SR-IOV acceleration
- **Compatibility**: Works with standard networking applications

## Prerequisites

- SR-IOV capable network interface
- SR-IOV Network Operator installed and configured
- At least one Virtual Function available
- DRA-enabled Kubernetes cluster (v1.26+)
- Appropriate node labeling and device plugin setup

## Common Use Patterns

This basic setup serves as foundation for:
- Migrating applications to SR-IOV
- Performance testing and benchmarking
- Learning SR-IOV concepts and troubleshooting
- Building more complex multi-interface configurations
