# --------------------------------------------------------------------------------
# S3 based terraform remote state setup, uncomment and complete to use the mentio-
# ned bucket and table for remote state store.
# --------------------------------------------------------------------------------
# terraform {
#   required_version = ">= 0.12"

#   backend "s3" {
#     bucket         = ""
#     dynamodb_table = ""
#     key            = "dgraph/terraform_state"
#     region         = "ap-southeast-1"
#     encrypt        = true
#   }
# }

# --------------------------------------------------------------------------------
# Setup AWS provider
# --------------------------------------------------------------------------------
provider "aws" {
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
  region     = var.region
  profile    = var.profile
}

locals {
  deployment_name = "${var.service_prefix}${var.deployment_name}"
}

# --------------------------------------------------------------------------------
# Setup Dgraph module to create the cluster with dgraph running
# --------------------------------------------------------------------------------
module "dgraph" {
  source = "./dgraph"

  name   = local.deployment_name
  ami_id = var.ami_id

  alpha_count = var.alpha_count
  zero_count  = var.zero_count

  alpha_instance_type = var.alpha_instance_type
  zero_instance_type  = var.zero_instance_type

  alpha_disk_size = var.alpha_disk_size
  zero_disk_size  = var.zero_disk_size
  disk_iops       = var.disk_iops

  key_pair_name = var.key_pair_name
  public_key    = var.public_key

  cidr_block                  = var.vpc_cidr_block
  subnet_cidr_block           = var.vpc_subnet_cidr_block
  secondary_subnet_cidr_block = var.vpc_secondary_subnet_cidr_block

  dgraph_version = var.dgraph_version
}
