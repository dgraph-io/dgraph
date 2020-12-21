variable "resource_group_name" {}
variable "storage_account_name" {}
variable "storage_container_name" { default = "dgraph-backups" }
variable "create_minio_env" { default = true }
variable "create_minio_secrets" { default = true }
variable "create_dgraph_secrets" { default = true }

## Create a Resource Group, a Storage Account, and a Storage Container
module "dgraph_backups" {
  source                 = "git::https://github.com/darkn3rd/simple-azure-blob.git?ref=v0.1"
  resource_group_name    = var.resource_group_name
  create_resource_group  = true
  storage_account_name   = var.storage_account_name
  create_storage_account = true
  storage_container_name = var.storage_container_name
}

#####################################################################
# Locals
#####################################################################

locals {
  minio_vars = {
    accessKey = module.dgraph_backups.AccountName
    secretKey = module.dgraph_backups.AccountKey
  }

  dgraph_secrets = templatefile("${path.module}/templates/dgraph_secrets.yaml.tmpl", local.minio_vars)
  minio_secrets  = templatefile("${path.module}/templates/minio_secrets.yaml.tmpl", local.minio_vars)
  minio_env      = templatefile("${path.module}/templates/minio.env.tmpl", local.minio_vars)
}

#####################################################################
# File Resources
#####################################################################
resource "local_file" "minio_env" {
  count           = var.create_minio_env != "" ? 1 : 0
  content         = local.minio_env
  filename        = "${path.module}/../minio.env"
  file_permission = "0644"
}

resource "local_file" "minio_secrets" {
  count           = var.create_minio_secrets != "" ? 1 : 0
  content         = local.minio_secrets
  filename        = "${path.module}/../charts/minio_secrets.yaml"
  file_permission = "0644"
}

resource "local_file" "dgraph_secrets" {
  count           = var.create_dgraph_secrets != "" ? 1 : 0
  content         = local.dgraph_secrets
  filename        = "${path.module}/../charts/dgraph_secrets.yaml"
  file_permission = "0644"
}
