variable "name" {}
variable "project_id" {}
variable "zone" { default = "us-central1-b" }
variable "tier" { default = "STANDARD" }
variable "network" { default = "default" }
variable "capacity_gb" { default = 1024 }
variable "share_name" { default = "volumes" }
variable "create_env_sh" { default = true }

#####################################################################
# Google Cloud Filestore instance
#####################################################################
module "gcfs" {
  source      = "./modules/simple_gcfs"
  name        = var.name
  zone        = var.zone
  tier        = var.tier
  network     = var.network
  capacity_gb = var.capacity_gb
  share_name  = var.share_name
}

#####################################################################
# Locals
#####################################################################
locals {
  env_vars = {
    nfs_server = module.gcfs.nfs_server
    nfs_path   = module.gcfs.nfs_path
  }

  env_sh = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}

#####################################################################
# Create ../env.sh
#####################################################################
resource "local_file" "env_sh" {
  count           = var.create_env_sh != "" ? 1 : 0
  content         = local.env_sh
  filename        = "${path.module}/../env.sh"
  file_permission = "0644"
}
