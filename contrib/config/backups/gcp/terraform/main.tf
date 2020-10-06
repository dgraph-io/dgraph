#####################################################################
# Variables
#####################################################################
variable "region" {}
variable "project_id" {}
variable "name" {}
variable "create_minio_env" { default = true }
variable "create_minio_secrets" { default = true }
variable "create_dgraph_secrets" { default = true }
variable "create_credentials_json" { default = true }
variable "create_env_sh" { default = true }
variable "minio_access_key" { default = "" }
variable "minio_secret_key" { default = "" }

#####################################################################
# Modules
#####################################################################
module "dgraph_backups" {
  source     = "git::https://github.com/terraform-google-modules/terraform-google-cloud-storage.git//modules/simple_bucket?ref=v1.7.0"
  name       = var.name
  project_id = var.project_id
  location   = var.region

  lifecycle_rules = [{
    action = {
      type = "Delete"
    }

    condition = {
      age        = 365
      with_state = "ANY"
    }
  }]
}

module "service_account" {
  source             = "./modules/gsa"
  service_account_id = var.name
  display_name       = var.name
  project_id         = var.project_id
  roles              = ["roles/storage.admin"]
}

#####################################################################
# Resources - Random Vars
#####################################################################
resource "random_string" "key" {
  length  = 20
  special = false
}

resource "random_password" "secret" {
  length = 40
}

#####################################################################
# Locals
#####################################################################
locals {
  minio_access_key = var.minio_access_key != "" ? var.minio_access_key : random_string.key.result
  minio_secret_key = var.minio_secret_key != "" ? var.minio_secret_key : random_password.secret.result

  minio_vars = {
    gcsKeyJson = indent(2, module.service_account.key)
    accessKey  = local.minio_access_key
    secretKey  = local.minio_secret_key
  }

  env_vars = {
    project_id = var.project_id
    bucket     = var.name
  }

  dgraph_secrets = templatefile("${path.module}/templates/dgraph_secrets.yaml.tmpl", local.minio_vars)
  minio_secrets  = templatefile("${path.module}/templates/minio_secrets.yaml.tmpl", local.minio_vars)
  minio_env      = templatefile("${path.module}/templates/minio.env.tmpl", local.minio_vars)
  env_sh         = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}

#####################################################################
# Resources - Files
#####################################################################
resource "local_file" "credentials" {
  count           = var.create_credentials_json ? 1 : 0
  content         = module.service_account.key
  filename        = "${path.module}/../credentials.json"
  file_permission = "0644"
}

resource "local_file" "minio_env" {
  count           = var.create_minio_env != "" ? 1 : 0
  content         = local.minio_env
  filename        = "${path.module}/../minio.env"
  file_permission = "0644"
}

resource "local_file" "env_sh" {
  count           = var.create_env_sh != "" ? 1 : 0
  content         = local.env_sh
  filename        = "${path.module}/../env.sh"
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
