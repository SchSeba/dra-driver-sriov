#!/usr/bin/env bash
set -xeo pipefail

source hack/common.sh

NUM_OF_WORKERS=${NUM_OF_WORKERS:-0}
total_number_of_nodes=$((1 + NUM_OF_WORKERS))

## Global configuration
export NAMESPACE="sriov-network-operator"
export OPERATOR_NAMESPACE="sriov-network-operator"
export OPERATOR_EXEC=kubectl
export CLUSTER_HAS_EMULATED_PF=TRUE

export MULTUS_NAMESPACE="kube-system"

check_requirements() {
  for cmd in kcli virsh virt-edit podman make go; do
    if ! command -v "$cmd" &> /dev/null; then
      echo "$cmd is not available"
      exit 1
    fi
  done
  return 0
}

echo "## checking requirements"
check_requirements
echo "## delete existing cluster name $cluster_name"
kcli delete cluster $cluster_name -y
kcli delete network $cluster_name -y

function cleanup {
  kcli delete cluster $cluster_name -y
  kcli delete network $cluster_name -y
}

if [ -z $SKIP_DELETE ]; then
  trap cleanup EXIT
fi

kcli create network -c 192.168.120.0/24 ${network_name}
kcli create network -c 192.168.${virtual_router_id}.0/24 --nodhcp -i $cluster_name

# TODO: remove this once we have a newer engine version
cat <<EOF > ./${cluster_name}-plan.yaml
version: $cluster_version
ctlplane_memory: 4096
worker_memory: 4096
pool: default
disk_size: 50
network: ${network_name}
api_ip: $api_ip
virtual_router_id: $virtual_router_id
domain: $domain_name
ctlplanes: 1
workers: $NUM_OF_WORKERS
ingress: false
machine: q35
engine: crio
sdn: flannel
autolabeller: false
vmrules:
  - $cluster_name-ctlplane-.*:
      nets:
        - name: ${network_name}
          type: igb
          vfio: true
          noconf: true
        - name: $cluster_name
          type: igb
          vfio: true
          noconf: true
  - $cluster_name-worker-.*:
      nets:
        - name: ${network_name}
          type: igb
          vfio: true
          noconf: true
          numa: 0
        - name: $cluster_name
          type: igb
          vfio: true
          noconf: true
          numa: 1
      numcpus: 6
      numa:
        - id: 0
          vcpus: 0,2,4
          memory: 2048
        - id: 1
          vcpus: 1,3,5
          memory: 2048

EOF

kcli create cluster generic --paramfile ./${cluster_name}-plan.yaml $cluster_name

export KUBECONFIG=$HOME/.kcli/clusters/$cluster_name/auth/kubeconfig
export PATH=$PWD:$PATH

ATTEMPTS=0
MAX_ATTEMPTS=72
ready=false
sleep_time=10

until $ready || [ $ATTEMPTS -eq $MAX_ATTEMPTS ]
do
    echo "waiting for cluster to be ready"
    if [ `kubectl get node | grep Ready | wc -l` == $total_number_of_nodes ]; then
        echo "cluster is ready"
        ready=true
    else
        echo "cluster is not ready yet"
        sleep $sleep_time
    fi
    ATTEMPTS=$((ATTEMPTS+1))
done

if ! $ready; then
    echo "Timed out waiting for cluster to be ready"
    kubectl get nodes
    exit 1
fi

function update_worker_labels() {
echo "## label cluster workers as sriov capable"
for ((num=0; num<NUM_OF_WORKERS; num++))
do
    kubectl label node $cluster_name-worker-$num.$domain_name feature.node.kubernetes.io/network-sriov.capable=true --overwrite
done

echo "## label cluster worker as worker"
for ((num=0; num<NUM_OF_WORKERS; num++))
do
  kubectl label node $cluster_name-worker-$num.$domain_name node-role.kubernetes.io/worker= --overwrite
done
}

update_worker_labels

controller_ip=`kubectl get node -o wide | grep ctlp | awk '{print $6}'`
insecure_registry="[[registry]]
location = \"$controller_ip:5000\"
insecure = true

[aliases]
\"golang\" = \"docker.io/library/golang\"
"

cat << EOF > /etc/containers/registries.conf.d/003-${cluster_name}.conf
$insecure_registry
EOF

function update_host() {
    node_name=$1
    kcli ssh $node_name << EOF
sudo su
echo '$insecure_registry' > /etc/containers/registries.conf.d/003-internal.conf
systemctl restart crio

echo '[connection]
id=multi
type=ethernet
[ethernet]
[match]
driver=igbvf;
[ipv4]
method=disabled
[ipv6]
addr-gen-mode=default
method=disabled
[proxy]' > /etc/NetworkManager/system-connections/multi.nmconnection

chmod 600 /etc/NetworkManager/system-connections/multi.nmconnection

echo '[Unit]
Description=disable checksum offload to avoid vf bug
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/bin/bash -c "ethtool --offload  eth1  rx off  tx off && ethtool -K eth1 gso off"
StandardOutput=journal+console
StandardError=journal+console

[Install]
WantedBy=default.target' > /etc/systemd/system/disable-offload.service

systemctl daemon-reload
systemctl enable --now disable-offload

echo '[Unit]
Description=load br_netfilter
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/bin/bash -c "modprobe br_netfilter"
StandardOutput=journal+console
StandardError=journal+console

[Install]
WantedBy=default.target' > /etc/systemd/system/load-br-netfilter.service

systemctl daemon-reload
systemctl enable --now load-br-netfilter

systemctl restart NetworkManager

grubby --update-kernel=DEFAULT --args=pci=realloc
grubby --update-kernel=DEFAULT --args=iommu=pt
grubby --update-kernel=DEFAULT --args=intel_iommu=on

EOF
}

update_host $cluster_name-ctlplane-0
for ((num=0; num<NUM_OF_WORKERS; num++))
do
  update_host $cluster_name-worker-$num
  kcli ssh $cluster_name-worker-$num sudo reboot
done

# after the reboot, wait for the nodes to be ready
kubectl wait --for=condition=ready node --all --timeout=10m

# remove the patch after multus bug is fixed
# https://github.com/k8snetworkplumbingwg/multus-cni/issues/1221
kubectl patch  -n ${MULTUS_NAMESPACE} ds/kube-multus-ds --type=json -p='[{"op": "replace", "path": "/spec/template/spec/initContainers/0/command", "value":["cp", "-f","/usr/src/multus-cni/bin/multus-shim", "/host/opt/cni/bin/multus-shim"]}]'

kubectl -n ${MULTUS_NAMESPACE} get po | grep multus | awk '{print "kubectl -n kube-system delete po",$1}' | sh
kubectl -n kube-system get po | grep coredns | awk '{print "kubectl -n kube-system delete po",$1}' | sh

TIMEOUT=400
echo "## wait for coredns"
kubectl -n kube-system wait --for=condition=available deploy/coredns --timeout=${TIMEOUT}s
echo "## wait for multus"
kubectl -n ${MULTUS_NAMESPACE} wait --for=condition=ready -l name=multus pod --timeout=${TIMEOUT}s

echo "## deploy cert manager"
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml

echo "## wait for cert manager to be ready"

ATTEMPTS=0
MAX_ATTEMPTS=72
ready=false
sleep_time=5

until $ready || [ $ATTEMPTS -eq $MAX_ATTEMPTS ]
do
    echo "waiting for cert manager webhook to be ready"
    if [ `kubectl -n cert-manager get po | grep webhook | grep "1/1" | wc -l` == 1 ]; then
        echo "cluster is ready"
        ready=true
    else
        echo "cert manager webhook is not ready yet"
        sleep $sleep_time
    fi
    ATTEMPTS=$((ATTEMPTS+1))
done

echo "## Cluster deployed successfully"
