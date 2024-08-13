#####################################################################
# Variables
#####################################################################
variable "region" {}
variable "name" {}
variable "user_enabled" { default = true }
variable "create_env_sh" { default = true }
variable "create_s3_env" { default = true }
variable "create_dgraph_secrets" { default = true }

#####################################################################
# Bucket Module
#####################################################################
module "bucket" {
  source       = "github.com/darkn3rd/s3-bucket?ref=v1.0.0"
  name         = var.name
  user_enabled = var.user_enabled
}

#####################################################################
# Locals
#####################################################################

locals {
  s3_vars = {
    access_key_id     = module.bucket.access_key_id
    secret_access_key = module.bucket.secret_access_key
  }

  env_vars = {
    bucket_region = var.region
    bucket_name   = var.name
  }

  dgraph_secrets = templatefile("${path.module}/templates/dgraph_secrets.yaml.tmpl", local.s3_vars)
  env_sh         = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
  s3_env         = templatefile("${path.module}/templates/s3.env.tmpl", local.s3_vars)
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

resource "local_file" "s3_env" {
  count           = var.create_s3_env != "" ? 1 : 0
  content         = local.s3_env
  filename        = "${path.module}/../s3.env"
  file_permission = "0644"
}

resource "local_file" "dgraph_secrets" {
  count           = var.create_dgraph_secrets != "" ? 1 : 0
  content         = local.dgraph_secrets
  filename        = "${path.module}/../charts/dgraph_secrets.yaml"
  file_permission = "0644"
}
