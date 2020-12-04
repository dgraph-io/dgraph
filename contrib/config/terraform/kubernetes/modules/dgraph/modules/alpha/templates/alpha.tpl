#//usr/bin/env bash

set -ex

# TODO: Use zero_port variable when kubernetes_service resource of Terraform supports outputs
dgraph alpha --my=$(hostname -f):7080 --zero ${zero_address}:5080
