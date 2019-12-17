#!/usr/bin/env bash

readonly stack_name="${1}"
readonly region="${2:-$(aws configure get region)}"

readonly template="dgraph.json"
readonly ssh_key_name="dgraph-cloudformation-deployment-key"

aws cloudformation deploy \
    --capabilities CAPABILITY_IAM \
    --template-file "${template}" \
    --s3-bucket "dgraph-marketplace-cf-template-${region}" \
    --stack-name "${stack_name}" \
    --region "${region}" \
    --parameter-overrides \
        KeyName="${ssh_key_name}"
