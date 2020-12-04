#!/usr/bin/env bash

#####
# main
##################
main() {
  check_environment $@
  create_resource_group
  create_storage_acct
  authorize_ad_user
  create_storage_container
  create_config_files
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

  ## Defaults
  MY_CONTAINER_NAME=${MY_CONTAINER_NAME:-$1}
  MY_STORAGE_ACCT=${MY_STORAGE_ACCT:-""}
  MY_RESOURCE_GROUP=${MY_RESOURCE_GROUP:=""}
  MY_LOCATION=${MY_LOCATION:-"eastus2"}
  MY_ACCOUNT_ID="$(az account show | jq '.id' -r)"
  CREATE_MINIO_ENV=${CREATE_MINIO_ENV:-"true"}
  CREATE_MINIO_CHART_SECRETS=${CREATE_MINIO_CHART_SECRETS:-"true"}
  CREATE_DGRAPH_CHART_SECRETS=${CREATE_DGRAPH_CHART_SECRETS:-"true"}

  if [[ -z "${MY_CONTAINER_NAME}" ]]; then
    if (( $# < 1 )); then
      printf "[ERROR]: Need at least one parameter or define 'MY_CONTAINER_NAME'\n\n" 1>&2
      printf "Usage:\n\t$0 <container-name>\n\tMY_CONTAINER_NAME=<container-name> $0\n" 1>&2
      exit 1
    fi
  fi

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
# create_resource_group
##################
create_resource_group() {
  ## create resource (idempotently)
  if ! az group list | jq '.[].name' -r | grep -q ${MY_RESOURCE_GROUP}; then
    echo "[INFO]: Creating Resource Group '${MY_RESOURCE_GROUP}' at Location '${MY_LOCATION}'"
    az group create --name=${MY_RESOURCE_GROUP} --location=${MY_LOCATION} > /dev/null
  fi
}

#####
# create_storage_acct
##################
create_storage_acct() {
  ## create globally unique storage account (idempotently)
  if ! az storage account list | jq '.[].name' -r | grep -q ${MY_STORAGE_ACCT}; then
    echo "[INFO]: Creating Storage Account '${MY_STORAGE_ACCT}'"
    az storage account create \
      --name ${MY_STORAGE_ACCT} \
      --resource-group ${MY_RESOURCE_GROUP} \
      --location ${MY_LOCATION} \
      --sku Standard_ZRS \
      --encryption-services blob > /dev/null
  fi
}

#####
# authorize_ad_user
##################
authorize_ad_user() {
  ## Use Azure AD Account to Authorize Operation
  az ad signed-in-user show --query objectId -o tsv | az role assignment create \
      --role "Storage Blob Data Contributor" \
      --assignee @- \
      --scope "/subscriptions/${MY_ACCOUNT_ID}/resourceGroups/${MY_RESOURCE_GROUP}/providers/Microsoft.Storage/storageAccounts/${MY_STORAGE_ACCT}" > /dev/null
}

#####
# create_storage_container
##################
create_storage_container() {
  ## Create Container Using Credentials
  if ! az storage container list \
   --account-name ${MY_STORAGE_ACCT} \
   --auth-mode login | jq '.[].name' -r | grep -q ${MY_CONTAINER_NAME}
  then
    echo "[INFO]: Creating Storage Container '${MY_CONTAINER_NAME}'"
    az storage container create \
      --account-name ${MY_STORAGE_ACCT} \
      --name ${MY_CONTAINER_NAME} \
      --auth-mode login > /dev/null
  fi
}

#####
# create_config_files
##################
create_config_files() {
  ## Create Minio  env file and Helm Chart secret files
  if [[ "${CREATE_MINIO_ENV}" =~ true|(y)es ]]; then
    echo "[INFO]: Creating Docker Compose 'minio.env' file"
    ./create_secrets.sh minio_env
  fi

  if [[ "${CREATE_MINIO_CHART_SECRETS}" =~ true|(y)es ]]; then
    echo "[INFO]: Creating Helm Chart 'minio_secrets.yaml' file"
    ./create_secrets.sh minio_chart
  fi

  if [[ "${CREATE_DGRAPH_CHART_SECRETS}" =~ true|(y)es ]]; then
    echo "[INFO]: Creating Helm Chart 'dgraph_secrets.yaml' file"
    ./create_secrets.sh dgraph_chart
  fi
}

main $@
