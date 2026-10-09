#!/usr/bin/env bash

cluster_version=${CLUSTER_VERSION:-1.36.1}
cluster_name=${CLUSTER_NAME:-dra}
domain_name=$cluster_name.lab
network_name=${NETWORK_NAME:-dra}

api_ip=${API_IP:-192.168.120.250}
virtual_router_id=${VIRTUAL_ROUTER_ID:-200}

export NAMESPACE="dra-driver-sriov"
export MULTUS_NAMESPACE="kube-system"
export KUBECONFIG=/root/.kcli/clusters/$cluster_name/auth/kubeconfig

export DRA_DRIVER_MODE=${DRA_DRIVER_MODE:-STANDALONE}

here="$(dirname "$(readlink --canonicalize "${BASH_SOURCE[0]}")")"
root="$(readlink --canonicalize "$here/..")"

check_requirements() {
  local -a cmds=(kcli virsh podman make go kubectl git jq)
  for cmd in "${cmds[@]}"; do
    if ! command -v "$cmd" &> /dev/null; then
      echo "$cmd is not available"
      exit 1
    fi
  done
}

# Resolve the control-plane InternalIP (internal registry host).
get_controller_ip() {
  controller_ip=$(kubectl get node "${cluster_name}-ctlplane-0.${domain_name}" \
    -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
  if [[ -z "$controller_ip" ]]; then
    echo "## ERROR: Failed to get controller IP"
    kubectl get nodes -o wide
    exit 1
  fi
}
