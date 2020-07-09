##
## Variables
variable "eks_cluster_name" {}
variable "region" {}
variable "instance_type" {
  default = "m5.large"
}
variable "workers_additional_policies" {
  default = []
}

##
## Data Sources
data "aws_availability_zones" "available" {}

##
## VPC Module 
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "2.44.0"

  name                 = "ekstf-vpc"
  cidr                 = "192.168.0.0/16"
  azs                  = data.aws_availability_zones.available.names
  private_subnets      = ["192.168.160.0/19", "192.168.128.0/19", "192.168.96.0/19"]
  public_subnets       = ["192.168.64.0/19", "192.168.32.0/19", "192.168.0.0/19"]
  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true

  tags = {
    "kubernetes.io/cluster/${var.eks_cluster_name}" = "shared",
  }

  public_subnet_tags = {
    "kubernetes.io/cluster/${var.eks_cluster_name}" = "shared"
    "kubernetes.io/role/elb"                        = "1"
  }

  private_subnet_tags = {
    "kubernetes.io/cluster/${var.eks_cluster_name}" = "shared"
    "kubernetes.io/role/internal-elb"               = "1"
  }
}

##
## EKS Module 
module "eks" {
  source       = "terraform-aws-modules/eks/aws"
  version      = "12.1.0"
  cluster_name = var.eks_cluster_name
  subnets      = module.vpc.private_subnets
  vpc_id       = module.vpc.vpc_id

  workers_additional_policies = var.workers_additional_policies

  worker_groups = [
    {
      name                 = "worker-group-1"
      instance_type        = var.instance_type
      asg_desired_capacity = 2
    }
  ]
}
