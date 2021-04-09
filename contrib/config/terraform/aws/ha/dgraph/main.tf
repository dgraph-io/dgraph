locals {
  deployment_name = "${var.name}-dgraph"
}

resource "aws_key_pair" "dgraph_key" {
  key_name   = var.key_pair_name
  public_key = var.public_key
}

module "aws_vpc" {
  source = "./../aws/vpc"

  name              = local.deployment_name
  cidr_block        = var.cidr_block
  subnet_cidr_block = var.subnet_cidr_block

  secondary_subnet_cidr_block = var.secondary_subnet_cidr_block
}

module "aws_lb" {
  source = "./../aws/load_balancer"

  deployment_name     = local.deployment_name
  subnet_id           = module.aws_vpc.subnet_id
  secondary_subnet_id = module.aws_vpc.secondary_subnet_id
  sg_id               = module.aws_vpc.alb_sg_id
}

module "zero" {
  source = "./zero"

  ami_id = var.ami_id

  name           = local.deployment_name
  vpc_id         = module.aws_vpc.vpc_id
  sg_id          = module.aws_vpc.sg_id
  instance_count = var.zero_count

  subnet_id = module.aws_vpc.subnet_id
  lb_arn    = module.aws_lb.arn

  instance_type = var.zero_instance_type
  disk_size     = var.zero_disk_size
  disk_iops     = var.disk_iops

  key_pair_name     = var.key_pair_name
  subnet_cidr_block = var.subnet_cidr_block

  dgraph_version = var.dgraph_version
}

module "alpha" {
  source = "./alpha"

  ami_id = var.ami_id

  name           = local.deployment_name
  vpc_id         = module.aws_vpc.vpc_id
  sg_id          = module.aws_vpc.sg_id
  instance_count = var.alpha_count

  subnet_id = module.aws_vpc.subnet_id
  lb_arn    = module.aws_lb.arn

  instance_type = var.alpha_instance_type
  disk_size     = var.alpha_disk_size
  disk_iops     = var.disk_iops

  key_pair_name   = var.key_pair_name
  healthy_zero_ip = module.zero.healthy_zero_ip

  dgraph_version = var.dgraph_version

  # We first initialize zeros and then alphas because for starting alphas
  # we need the address of a healthy zero.
  # Terraform 0.12 does not support depends_on, use this later on.
  # depends_on = [module.zero]
}
