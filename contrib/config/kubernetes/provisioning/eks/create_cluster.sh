#!/usr/bin/env bash

## Check for aws command
command -v aws > /dev/null || \
  { echo 'aws command not not found' >&2; exit 1; }

## Check for eksctl command
command -v eksctl > /dev/null || \
  { echo 'aws command not not found' >&2; exit 1; }

## Usage
if [[ $1 =~ h(elp)?|\? ]]; then
  printf "   Usage: \n\tCLUSTER_NAME=my_cluster\n\tINSTANCE_TYPE=m5.large\n\tREGION=us-east-2\n\tVERSION=1.17\n\t$0 \n\n" >&2
  exit 1
fi

## Edit New Temporary File
OPTIONS=""

if [[ ! -z "$VERSION" ]]; then
  OPTIONS="$OPTIONS -e \"s/(version:).*(\\\".*\\\")/\1 \\\"$VERSION\\\"/\""
fi

if [[ ! -z "$REGION" ]]; then
  OPTIONS="$OPTIONS -e \"s/us-east-2/$REGION/\""
fi

if [[ ! -z "$INSTANCE_TYPE" ]]; then
  OPTIONS="$OPTIONS -e \"s/m5\.large/$INSTANCE_TYPE/\""
fi

if [[ ! -z "$CLUSTER_NAME" ]]; then
  OPTIONS="$OPTIONS -e \"s/dgraph-ha-cluster/$CLUSTER_NAME/\""
fi

## Create Temp File to Use
if [[ ! -z "$OPTIONS" ]]; then
  eval "sed -E $OPTIONS cluster.yaml > temp.yaml"
else
  cp cluster.yaml temp.yaml
fi

## Create EKS Cluster
eksctl create cluster --config-file temp.yaml
