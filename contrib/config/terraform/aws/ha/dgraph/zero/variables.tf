variable "name" {
  type        = string
  description = "Name of the dgraph deployment"
}

variable "instance_count" {
  type        = number
  description = "Number of dgraph zeros to run in the cluster."
}

variable "instance_type" {
  type        = string
  description = "EC2 Instance type for dgraph zero component."
}

variable "disk_size" {
  type        = string
  description = "Disk size for dgraph zero node."
}

variable "disk_iops" {
  type        = number
  description = "IOPS limit for the disk associated with the instance."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID of the dgraph cluster we created."
}

variable "lb_arn" {
  type        = string
  description = "Resource ARN of the dgraph load balancer."
}

variable "sg_id" {
  type        = string
  description = "Security group ID for the created dgraph VPC."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID within VPC for dgraph deployment."
}

variable "subnet_cidr_block" {
  type        = string
  description = "CIDR block corresponding to the dgraph subnet."
}

variable "ami_id" {
  type        = string
  description = "AMI to use for the instances"
}

variable "key_pair_name" {
  type        = string
  description = "Key Pair name to associate with the instances."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation."
}
