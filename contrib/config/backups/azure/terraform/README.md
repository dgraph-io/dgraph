# Azure Blob with Terraform

## About

This script will create the required resources needed to create Azure Blob Storage using [`simple-azure-blob`](https://github.com/darkn3rd/simple-azure-blob) module.

## Prerequisites

You need the following installed to use this automation:

* [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest) with an active Azure subscription configured.
* [Terraform](https://www.terraform.io/downloads.html) tool that is used to provision resources and create configuration files from templates

## Configuration

You will need to define the following variables:

* Required Variables:
  * `resource_group_name` (required) - Azure resource group that contains the resources
  * `storage_account_name` (required) - Azure storage account (unique global name) to contain storage
  * `storage_container_name` (default = `dgraph-backups`) - Azure container to host the blob storage

## Steps

### Define Variables

You can define these when prompted, or in `terrafrom.tfvars` file, or through command line variables, e.g. `TF_VAR_resource_group_name`, `TF_VAR_storage_account_name`.

```terraform
# terraform.tfvars
resource_group_name  = "my-organization-resources"
storage_account_name = "myorguniquestorage12345"
```

### Download Plugins and Modules

```bash
terraform init
```

### Prepare and Provision Resources

```bash
## get a list of changes that will be made
terraform plan
## apply the changes
terraform apply
```

## Cleanup

```bash
terraform destroy
```
