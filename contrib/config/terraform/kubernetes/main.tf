terraform {
  required_version = ">= 0.12.0"
}

data "http" "localip" {
  url = "http://ipv4.icanhazip.com"
}

locals {
  whitelisted_cidrs = var.only_whitelist_local_ip ? ["${chomp(data.http.localip.body)}/32"] : var.ingress_whitelist_cidrs
}

module "aws" {
  source = "./modules/aws"

  prefix = var.prefix
  cidr   = var.cidr
  region = var.region

  ha                      = var.ha
  worker_nodes_count      = var.worker_nodes_count
  instance_types          = var.instance_types
  ingress_whitelist_cidrs = local.whitelisted_cidrs
}

module "dgraph" {
  source = "./modules/dgraph"

  prefix          = var.prefix
  ha              = var.ha
  namespace       = var.namespace
  kubeconfig_path = module.aws.kubeconfig_path

  zero_replicas        = var.zero_replicas
  zero_persistence     = var.zero_persistence
  zero_storage_size_gb = var.zero_storage_size_gb

  alpha_initialize_data = var.alpha_initialize_data
  alpha_replicas        = var.alpha_replicas
  alpha_persistence     = var.alpha_persistence
  alpha_storage_size_gb = var.alpha_storage_size_gb
  alpha_lru_size_mb     = var.alpha_lru_size_mb
  # The Kubernetes Service Terraform resource does not expose any attributes
  zero_address = "${var.prefix}-dgraph-zero-0.${var.prefix}-dgraph-zero.${var.namespace}.svc.cluster.local"

  ingress_whitelist_cidrs = local.whitelisted_cidrs
}
