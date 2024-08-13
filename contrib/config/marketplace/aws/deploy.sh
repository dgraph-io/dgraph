#!/usr/bin/env bash

readonly stack_name="${1}"
readonly ssh_key_name="${2}"
readonly region="${3:-$(aws configure get region)}"
readonly s3_bucket_name="${4:-dgraph-marketplace-cf-stack-${stack_name}-${region}}"
readonly template="dgraph.json"

# validate arguments
[[ $# -lt 2 ]] && \
 { echo "Usage $0 STACK_NAME SSH_KEY_NAME [REGION] [S3_BUCKET_NAME]." &> /dev/stderr; exit 1; }
[[ -z $stack_name ]] && \
 { echo "Stack name not specified.  Exiting." &> /dev/stderr; exit 1; }
[[ -z $ssh_key_name ]] && \
 { echo "SSH Key Name not specified. Exiting." &> /dev/stderr; exit 1; }

# create required bucket if it doesn't exist
aws s3 ls --region ${region} "s3://${s3_bucket_name}" &> /dev/null || \
 aws s3 mb --region ${region} "s3://${s3_bucket_name}"

# create cfn stack
aws cloudformation deploy \
 --capabilities CAPABILITY_IAM \
 --template-file "${template}" \
 --s3-bucket "${s3_bucket_name}" \
 --stack-name "${stack_name}" \
 --region "${region}" \
 --parameter-overrides \
  KeyName="${ssh_key_name}"
