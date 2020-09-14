variable "resource_group_name" {}
variable "storage_account_name" {}
variable "storage_container_name" {}

## Create Resource Group, Storage Account Name, and Container
module "app_backups" {
  source                  = "git::https://github.com/darkn3rd/simple-azure-blob.git?ref=v0.1"
  resource_group_name     = var.resource_group_name
  create_resource_group   = true
  storage_account_name    = var.storage_account_name
  create_storage_account  = true
  storage_container_name  = "app-backups"
}
