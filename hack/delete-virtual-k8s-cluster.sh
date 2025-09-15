#!/usr/bin/env bash
set -xeo pipefail

source hack/common.sh

kcli delete cluster $cluster_name -y
kcli delete network $cluster_name -y