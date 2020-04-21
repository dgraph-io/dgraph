terraform {
  required_version = ">= 0.12.0"
}

module "vpc" {
  source = "./modules/vpc"

  cluster_name = var.prefix
  cidr         = var.cidr
  region       = var.region
}

module "eks" {
  source = "./modules/eks"

  cluster_name = var.prefix
  ha           = var.ha
  region       = var.region

  vpc_id                  = module.vpc.vpc_id
  cluster_subnet_ids      = module.vpc.cluster_subnet_ids
  db_subnet_ids           = module.vpc.db_subnet_ids
  worker_nodes_count      = var.worker_nodes_count
  instance_types          = var.instance_types
  ingress_whitelist_cidrs = var.ingress_whitelist_cidrs
}
