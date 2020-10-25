#####################################################################
# Variables
#####################################################################
variable "region" {}
variable "vpc_id" { default = "" }
variable "vpc_name" { default = "" }
variable "dns_name" {}
variable "encrypted" { default = false }
variable "create_env_sh" { default = true }
variable "dns_domain" {}
#####################################################################
# Locals
#####################################################################

locals {
  ## Use specified vpc_id or fetch by tag Name
  vpc_id = var.vpc_id != "" ? var.vpc_id : data.aws_vpc.vpc.id

  env_vars = {
    nfs_server = module.efs.host
    nfs_path   = "/"
  }

  env_sh = templatefile("${path.module}/templates/env.sh.tmpl", local.env_vars)
}

#####################################################################
# Datasources
#####################################################################
data "aws_vpc" "vpc" {
  tags = {
    Name = var.vpc_name
  }
}

data "aws_subnet_ids" "private" {
  vpc_id = local.vpc_id

  filter {
    name   = "tag:kubernetes.io/cluster/${var.vpc_name}"
    values = ["shared"]
  }

  filter {
    name   = "tag:kubernetes.io/role/internal-elb"
    values = [1]
  }
}

data "aws_route53_zone" "devtest" {
  name = var.dns_domain
}

data "aws_security_group" "eks_nodes" {
  filter {
    name = "tag:aws:eks:cluster-name"
    values = [
      "${var.vpc_name}"
    ]
  }

  filter {
    name = "tag:kubernetes.io/cluster/${var.vpc_name}"
    values = [
      "owned"
    ]
  }
}


#####################################################################
# Modules
#####################################################################
module "efs" {
  source          = "git::https://github.com/darkn3rd/terraform-aws-efs.git?ref=tags/0.19.0"
  namespace       = "dgraph"
  stage           = "test"
  name            = "fileserver"
  region          = var.region
  vpc_id          = local.vpc_id
  subnets         = data.aws_subnet_ids.private.ids
  security_groups = [data.aws_security_group.eks_nodes.id]
  zone_id         = data.aws_route53_zone.devtest.zone_id
  dns_name        = var.dns_name
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
