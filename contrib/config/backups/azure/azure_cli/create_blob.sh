#!/usr/bin/env bash

## Check for Azure CLI command
command -v az > /dev/null || \
  { echo "'az' command not not found" 1>&2; exit 1; }
command -v jq > /dev/null || \
  { echo "'jq' command not not found" 1>&2; exit 1; }

## Defaults
MY_CONTAINER_NAME=${MY_CONTAINER_NAME:-$1}

if [[ -z "${MY_CONTAINER_NAME}" ]]; then
  if (( $# < 1 )); then
    printf "ERROR: Need at least one parameter or define 'MY_CONTAINER_NAME'\n\n" 1>&2
    printf "Usage:\n\t$0 <container-name>\n\tMY_CONTAINER_NAME=<container-name> $0\n" 1>&2
    exit 1
  fi
fi

MY_STORAGE_ACCT=${MY_STORAGE_ACCT:-"$MY_CONTAINER_NAME"}
MY_RESOURCE_GROUP=${MY_RESOURCE_GROUP:="$MY_CONTAINER_NAME"}
MY_LOCATION=${MY_LOCATION:-"eastus2"}
MY_ACCOUNT_ID="$(az account show | jq '.id' -r)"

## create resource (idempotently)
if ! az group list | jq '.[].name' -r | grep -q ${MY_RESOURCE_GROUP}; then
  az group create --name=${MY_RESOURCE_GROUP} --location=${MY_LOCATION}
fi

## create globally unique storage account (idempotently)
if ! az storage account list | jq '.[].name' -r | grep -q ${MY_STORAGE_ACCT}; then
  az storage account create \
    --name ${MY_STORAGE_ACCT} \
    --resource-group ${MY_RESOURCE_GROUP} \
    --location ${MY_LOCATION} \
    --sku Standard_ZRS \
    --encryption-services blob
fi

## Use Azure AD Account to Authorize Operation
az ad signed-in-user show --query objectId -o tsv | az role assignment create \
    --role "Storage Blob Data Contributor" \
    --assignee @- \
    --scope "/subscriptions/${MY_ACCOUNT_ID}/resourceGroups/${MY_RESOURCE_GROUP}/providers/Microsoft.Storage/storageAccounts/${MY_STORAGE_ACCT}"

## Create Container Using Credentials
if ! az storage container list \
 --account-name ${MY_STORAGE_ACCT} \
 --auth-mode login | jq '.[].name' -r | grep -q ${MY_CONTAINER_NAME}
then
  az storage container create \
    --account-name ${MY_STORAGE_ACCT} \
    --name ${MY_CONTAINER_NAME} \
    --auth-mode login
fi
