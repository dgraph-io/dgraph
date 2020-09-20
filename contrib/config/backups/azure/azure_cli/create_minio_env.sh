#!/usr/bin/env bash

MY_STORAGE_ACCT=${MY_STORAGE_ACCT:-""}
MY_RESOURCE_GROUP=${MY_RESOURCE_GROUP:=""}

if [[ -z "${MY_STORAGE_ACCT}" ]]; then
  printf "ERROR: The env var of 'MY_STORAGE_ACCT' was not defined. Exiting\n" 1>&2
  exit 1
fi

if [[ -z "${MY_RESOURCE_GROUP}" ]]; then
  printf "ERROR: The env var of 'MY_RESOURCE_GROUP' was not defined. Exiting\n" 1>&2
  exit 1
fi

CONN_STR=$(az storage account show-connection-string \
    --name "${MY_STORAGE_ACCT}" \
    --resource-group "${MY_RESOURCE_GROUP}" \
     | jq .connectionString -r
)

cat <<-EOF > ../minio.env
MINIO_SECRET_KEY=$(grep -oP '(?<=AccountKey=).*' <<< $CONN_STR)
MINIO_ACCESS_KEY=$(grep -oP '(?<=AccountName=)[^;]*' <<< $CONN_STR)
EOF
