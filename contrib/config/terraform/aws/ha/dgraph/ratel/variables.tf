variable "name" {
  type        = string
  description = "Name of the dgraph deployment"
}

variable "instance_type" {
  type        = string
  description = "EC2 Instance type for dgraph ratel component."
}

variable "instance_count" {
  type        = number
  description = "Instance count for ratel."
  default     = 1
}

variable "disk_size" {
  type        = string
  description = "Disk size for dgraph ratel node."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID of the dgraph cluster we created."
}

variable "sg_id" {
  type        = string
  description = "Security group ID for the created dgraph VPC."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID within VPC for dgraph deployment."
}

variable "lb_arn" {
  type        = string
  description = "Resource ARN of the dgraph load balancer."
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

variable "alpha_completed" {
  type        = bool
  description = "Temporary variable to define dependency between ratel and alpha."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation."
}
