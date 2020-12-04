# Provisioning Azure Blob with Azure CLI

## About

This script will create the required resources needed to create Azure Blob Storage using (`simple-azure-blob`)[https://github.com/darkn3rd/simple-azure-blob] module.

## Prerequisites

You need the following installed to use this automation:

* [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest) with an active Azure subscription configured.
* [jq](https://stedolan.github.io/jq/) - command-line JSON process that makes it easy to parse JSON output from Azure CLI.
* [bash](https://www.gnu.org/software/bash/) - shell environment

## Configuration

You will need to define these environment variables:

* Required Variables:
  * `MY_RESOURCE_GROUP` (required) - Azure resource group that contains the resources. If the resource group does not exist, this script will create it.
  * `MY_STORAGE_ACCT` (required) - Azure storage account (unique global name) to contain storage.  If the storage account does not exist, this script will create it.
  * `MY_CONTAINER_NAME` (required) - Azure container to host the blob storage.  
* Optional Variables:
  * `MY_LOCATION` (default = `eastus2`)- the location where to create the resource group if it doesn't exist

## Steps

### Define Variables

You can create a `env.sh` with the desired values, for example:

```bash
cat <<-EOF > env.sh
export MY_RESOURCE_GROUP="my-organization-resources"
export MY_STORAGE_ACCT="myorguniquestorage12345"
export MY_CONTAINER_NAME="my-backups"
EOF
```

### Run the Script

```bash
## source env vars setup earlier
. env.sh
./create_blob.sh
```

## Cleanup

You can run these commands to delete the resources (with prompts) on Azure.

```bash
## source env vars setup earlier
. env.sh

if az storage account list | jq '.[].name' -r | grep -q ${MY_STORAGE_ACCT}; then
  az storage container delete \
    --account-name ${MY_STORAGE_ACCT} \
    --name ${MY_CONTAINER_NAME} \
    --auth-mode login

  az storage account delete \
    --name ${MY_STORAGE_ACCT} \
    --resource-group ${MY_RESOURCE_GROUP}
fi

if az group list | jq '.[].name' -r | grep -q ${MY_RESOURCE_GROUP}; then
  az group delete --name=${MY_RESOURCE_GROUP}
fi
```
