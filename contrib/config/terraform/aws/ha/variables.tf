variable "region" {
  type        = string
  default     = "us-east-2"
  description = "The region to deploy the EC2 instance in."
}

variable "profile" {
  type    = string
  default = "terraform"
}

variable "aws_access_key" {
  type        = string
  description = "Access key for the AWS account to create the dgraph deployment in."
}

variable "aws_secret_key" {
  type        = string
  description = "Secret key for the AWS account."
}

variable "deployment_name" {
  type        = string
  description = "Name of the deployment for dgraph, this is used in various places to tag the created resources."
}

variable "alpha_count" {
  type        = number
  description = "Number of dgraph alphas to run in the cluster, defaults to 3."
  default     = 3
}

variable "zero_count" {
  type        = number
  description = "Number of dgraph zeros to run in the cluster, defaults to 3."
  default     = 3
}

variable "alpha_instance_type" {
  type        = string
  description = "EC2 Instance type for dgraph alpha component."
  default     = "m5a.large"
}

variable "zero_instance_type" {
  type        = string
  description = "EC2 instance type for dgraph zero component."
  default     = "m5.large"
}

variable "alpha_disk_size" {
  type        = number
  description = "Disk size for the alpha node."
  default     = 500
}

variable "zero_disk_size" {
  type        = number
  description = "Disk size for dgraph zero node."
  default     = 250
}

variable "disk_iops" {
  type        = number
  description = "IOPS limit for the disk associated with the instance."
  default     = 1000
}

variable "service_prefix" {
  type        = string
  description = "Prefix to add in all the names and tags of EC2 components, defaults to empty"
  default     = ""
}

variable "vpc_cidr_block" {
  type        = string
  description = "CIDR block to assign to the VPC running the dgraph cluster, only used if a new VPC is created"
  default     = "10.200.0.0/16"
}

variable "vpc_subnet_cidr_block" {
  type        = string
  description = "CIDR block for the subnet to create within the VPC, this subnet will be used for dgraph deployment."
  default     = "10.200.200.0/24"
}

variable "vpc_secondary_subnet_cidr_block" {
  type        = string
  description = "Secondary CIDR block for the subnet to create within the VPC, this subnet will be used for dgraph deployment."
  default     = "10.200.201.0/24"
}

variable "ami_id" {
  type        = string
  description = "AMI to use for the instances"
  default     = "ami-0c55b159cbfafe1f0"
}

variable "key_pair_name" {
  type        = string
  description = "Name of the key pair to create for attaching to each instance."
  default     = "dgraph_ha_key"
}

variable "public_key" {
  type        = string
  description = "Public key corresponding to the key pair."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation."
  default     = "21.03.0"
}
