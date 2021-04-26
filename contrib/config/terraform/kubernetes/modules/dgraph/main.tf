terraform {
  required_version = ">= 0.12.0"
}

resource "kubernetes_namespace" "dgraph_namespace" {
  metadata {
    name = var.namespace
  }
}

module "zero" {
  source = "./modules/zero"

  prefix    = var.prefix
  ha        = var.ha
  namespace = var.namespace

  replicas                = var.zero_replicas
  persistence             = var.zero_persistence
  storage_size_gb         = var.zero_storage_size_gb
  ingress_whitelist_cidrs = var.ingress_whitelist_cidrs

  namespace_resource = kubernetes_namespace.dgraph_namespace
}

module "alpha" {
  source = "./modules/alpha"

  prefix    = var.prefix
  ha        = var.ha
  namespace = var.namespace

  initialize_data = var.alpha_initialize_data

  # The Kubernetes Service Terraform resource does not expose any attributes
  zero_address = var.zero_address

  replicas        = var.alpha_replicas
  persistence     = var.alpha_persistence
  storage_size_gb = var.alpha_storage_size_gb

  lru_size_mb = var.alpha_lru_size_mb

  ingress_whitelist_cidrs = var.ingress_whitelist_cidrs

  namespace_resource = kubernetes_namespace.dgraph_namespace
}
