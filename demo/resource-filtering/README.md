# Resource Filtering Demo

This demo showcases how to use `SriovResourceFilter` to filter and manage SR-IOV Virtual Functions based on various hardware and configuration criteria.

## Overview

This scenario demonstrates:
- Creating resource filters based on vendor IDs, Physical Function names, and other hardware attributes
- Setting up multiple resource configurations for different network interfaces
- Deploying a pod that uses filtered SR-IOV resources with specific network requirements

## Components

### 1. SriovResourceFilter
The `SriovResourceFilter` resource defines how to filter available SR-IOV devices:
- **nodeSelector**: Targets specific nodes (`dra-ctlplane-0.dra.lab` in this example)
- **configs**: Defines multiple resource configurations:
  - `eth0_resource`: Filters devices connected to eth0 Physical Function
  - `eth1_resource`: Filters devices connected to eth1 Physical Function
- **resourceFilters**: Specifies filtering criteria:
  - Vendor ID filtering (Intel: `8086`)
  - Physical Function name filtering (`pfNames`)
  - Optional filters for device IDs, PCI addresses, NUMA nodes, and drivers

### 2. Networking Setup
- Creates a dedicated namespace (`vf-test4`)
- Sets up NetworkAttachmentDefinition with SR-IOV CNI configuration
- Configures IPAM with host-local plugin for subnet `10.0.1.0/24`

### 3. Resource Allocation
- ResourceClaimTemplate requests exactly 1 VF from `eth1_resource`
- Uses CEL (Common Expression Language) to filter devices by resource name
- Specifies VfConfig with interface name and network attachment

### 4. Pod Deployment
- Deploys a toolbox pod with network capabilities
- Claims the filtered SR-IOV resource
- Runs with privileged networking capabilities (NET_ADMIN, NET_RAW)

## Usage

1. Apply the resource filter to make filtered resources available:
   ```bash
   kubectl apply -f resource-filter.yaml
   ```

2. The DRA driver will discover and filter SR-IOV devices based on the criteria
3. Pods can then claim resources using the filtered resource names
4. The pod will be scheduled on nodes where matching resources are available

## Key Features

- **Granular Filtering**: Filter by vendor, device ID, PCI address, PF name, NUMA node, or driver
- **Multi-Resource Support**: Configure multiple resource types on the same node
- **CEL Integration**: Use Common Expression Language for advanced resource selection
- **Network Integration**: Seamless integration with SR-IOV CNI plugin

## Prerequisites

- Kubernetes cluster with DRA (Dynamic Resource Allocation) support
- SR-IOV capable hardware
- SR-IOV Network Operator installed
- Physical Functions (eth0, eth1) configured on target nodes
