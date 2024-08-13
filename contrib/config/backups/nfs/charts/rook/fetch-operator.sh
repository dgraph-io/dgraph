#!/usr/bin/env bash

command -v git > /dev/null || \
  { echo "[ERROR]: 'git' command not not found" 1>&2; exit 1; }

ROOK_VERSION="v1.4.7"
DEST_PATH="${PWD}/$(dirname "${BASH_SOURCE[0]}")/rook-nfs-operator-kustomize/base"
TEMP_PATH=$(mktemp -d)

cd $TEMP_PATH
git clone --single-branch --branch $ROOK_VERSION https://github.com/rook/rook.git 2> /dev/null

for MANIFEST in common.yaml provisioner.yaml operator.yaml; do
  cp $TEMP_PATH/rook/cluster/examples/kubernetes/nfs/$MANIFEST $DEST_PATH
done
