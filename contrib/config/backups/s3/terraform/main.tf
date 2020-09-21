#####################################################################
# Variables
#####################################################################
variable "region" {}
variable "name" {}
variable "user_enabled" { default = true }
variable "create_env_sh" { default = true }
variable "create_dgraph_secrets" { default = true }

#####################################################################
# Bucket Module
#####################################################################
module "bucket" {
  source       = "github.com/darkn3rd/s3-bucket"
  name         = var.name
  user_enabled = var.user_enabled
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
  env_sh         = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}

#####################################################################
# File Resources
#####################################################################
resource "local_file" "env_sh" {
  count           = var.create_env_sh != "" ? 1 : 0
  content         = local.env_sh
  filename        = "${path.module}/../env.sh"
  file_permission = "0644"
}


resource "local_file" "dgraph_secrets" {
  count           = var.create_dgraph_secrets != "" ? 1 : 0
  content         = local.dgraph_secrets
  filename        = "${path.module}/../charts/dgraph_secrets.yaml"
  file_permission = "0644"
}
