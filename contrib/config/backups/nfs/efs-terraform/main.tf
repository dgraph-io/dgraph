#####################################################################
# Locals
#####################################################################

locals {
  ## Use specified vpc_id or search byh vpc tag name
  vpc_id = var.vpc_id != "" ? var.vpc_id : data.aws_vpc.vpc_by_name[0].id
  vpc_name = var.vpc_name != "" ? var.vpc_name : data.aws_vpc.vpc_by_id[0].tags["Name"]

  ## lookup zone_id if dns_domain is passed
  zone_id = var.dns_domain == "" ? var.zone_id : data.aws_route53_zone.devtest[0].zone_id

  ## use vpc tag name as eks cluster name if not specified (default behavior with eksctl)
  eks_cluster_name = var.eks_cluster_name != "" ? var.eks_cluster_name : local.vpc_name

  ## fetch list of private subnets in current VPC if list of subnet IDs not specified
  subnets = var.subnets != [] ? var.subnets : data.aws_subnet_ids.private[0].ids

  ## fetch EKS Node SG if list of SG IDs are not specified
  security_groups = var.security_groups == [] ? var.security_groups : [data.aws_security_group.eks_nodes[0].id]

  env_vars = {
    nfs_server = local.zone_id == "" ? module.efs.dns_name : module.efs.host
    nfs_path   = "/"
  }

  env_sh = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}

######################################################################
## Datasources
######################################################################

data "aws_vpc" "vpc_by_name" {
  count = var.vpc_name == "" ? 0 : 1

  tags = {
    Name = var.vpc_name
  }
}

data "aws_vpc" "vpc_by_id" {
  count = var.vpc_id == "" ? 0 : 1
  id = local.vpc_id
}

## fetch private subnets if subnets were not specified
data "aws_subnet_ids" "private" {
  count = var.subnets != [] ? 0 : 1
  vpc_id = local.vpc_id

  ## Search for Subnet used by specific EKS Cluster
  filter {
    name   = "tag:kubernetes.io/cluster/${local.eks_cluster_name}"
    values = ["shared"]
  }

  ## Search for Subnets used designated as private for EKS Cluster
  filter {
    name   = "tag:kubernetes.io/role/internal-elb"
    values = [1]
  }
}

## lookup zone if dns_domain specified 
data "aws_route53_zone" "devtest" {
  count = var.dns_domain == "" ? 0 : 1
  name = var.dns_domain
}

## lookup SG ID used for EKS Nodes if not specified
## NOTE: If created by eksctl, the SG will have this description:
##  EKS created security group applied to ENI that is attached to EKS 
##  Control Plane master nodes, as well as any managed workloads.
data "aws_security_group" "eks_nodes" {
  count = var.security_groups == [] ? 0 : 1

  filter {
    name = "tag:aws:eks:cluster-name"
    values = ["${local.eks_cluster_name}"]
  }

  filter {
    name = "tag:kubernetes.io/cluster/${local.eks_cluster_name}"
    values = ["owned"]
  }
}


#####################################################################
# Modules
#####################################################################
module "efs" {
  source          = "git::https://github.com/cloudposse/terraform-aws-efs.git?ref=tags/0.22.0"
  namespace       = "dgraph"
  stage           = "test"
  name            = "fileserver"
  region          = var.region
  vpc_id          = local.vpc_id
  subnets         = local.subnets
  security_groups = local.security_groups
  zone_id         = local.zone_id
  dns_name        = var.dns_name
  encrypted       = var.encrypted
}

######################################################################
## Create ../env.sh
######################################################################
resource "local_file" "env_sh" {
  count           = var.create_env_sh != "" ? 1 : 0
  content         = local.env_sh
  filename        = "${path.module}/../env.sh"
  file_permission = "0644"
}
