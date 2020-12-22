#!/usr/bin/env bash

#####
# main
##################
main() {
  check_environment $@

  ## Fetch Secrets from Azure
  get_secrets

  ## Create Configuration with Secrets
  case $1 in
    minio_env)
      create_minio_env
      ;;
    minio_chart)
      create_minio_secrets
      ;;
    dgraph_chart)
      create_dgraph_secrets
      ;;
  esac
}

#####
# check_environment
##################
check_environment() {
  ## Check for Azure CLI command
  command -v az > /dev/null || \
    { echo "[ERROR]: 'az' command not not found" 1>&2; exit 1; }
  command -v jq > /dev/null || \
    { echo "[ERROR]: 'jq' command not not found" 1>&2; exit 1; }

  MY_STORAGE_ACCT=${MY_STORAGE_ACCT:-""}
  MY_RESOURCE_GROUP=${MY_RESOURCE_GROUP:=""}

  if [[ -z "${MY_STORAGE_ACCT}" ]]; then
    printf "[ERROR]: The env var of 'MY_STORAGE_ACCT' was not defined. Exiting\n" 1>&2
    exit 1
  fi

  if [[ -z "${MY_RESOURCE_GROUP}" ]]; then
    printf "[ERROR]: The env var of 'MY_RESOURCE_GROUP' was not defined. Exiting\n" 1>&2
    exit 1
  fi
}

#####
# get_secrets
##################
get_secrets() {
  CONN_STR=$(az storage account show-connection-string \
      --name "${MY_STORAGE_ACCT}" \
      --resource-group "${MY_RESOURCE_GROUP}" \
       | jq .connectionString -r
  )

  export MINIO_SECRET_KEY=$(grep -oP '(?<=AccountKey=).*' <<< $CONN_STR)
  export MINIO_ACCESS_KEY=$(grep -oP '(?<=AccountName=)[^;]*' <<< $CONN_STR)
}

#####
# create_minio_env
##################
create_minio_env() {
  cat <<-EOF > ../minio.env
MINIO_SECRET_KEY=$(grep -oP '(?<=AccountKey=).*' <<< $CONN_STR)
MINIO_ACCESS_KEY=$(grep -oP '(?<=AccountName=)[^;]*' <<< $CONN_STR)
EOF
}

#####
# create_minio_secrets
##################
create_minio_secrets() {
  cat <<-EOF > ../charts/minio_secrets.yaml
accessKey: ${MINIO_ACCESS_KEY}
secretKey: ${MINIO_SECRET_KEY}
EOF
}

#####
# create_dgraph_secrets
##################
create_dgraph_secrets() {
  cat <<-EOF > ../charts/dgraph_secrets.yaml
backups:
  keys:
    minio:
      access: ${MINIO_ACCESS_KEY}
      secret: ${MINIO_SECRET_KEY}
EOF
}

main $@
