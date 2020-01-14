variable "deployment_name" {
  type        = string
  description = "Name to associate with the created instance."
}

variable "disk_size" {
  type        = string
  description = "Disk size to associate with the running instance."
}

variable "io_optimized" {
  type        = string
  description = "Should we attach an IO optimized disk to the instance."
  default     = "false"
}

variable "disk_iops" {
  type        = number
  description = "IOPS limit for the disk associated with the instance."
}

variable "instance_type" {
  type        = string
  description = "AWS instance type to launch."
}

variable "instance_count" {
  type        = number
  description = "Number of AWS instances to create."
}

variable "ami_id" {
  type        = string
  description = "AMI to launch the instance with."
}

variable "key_pair_name" {
  type        = string
  description = "AWS key-pair name to associate with the launched instance for SSH access"
}

variable "sg_id" {
  type        = string
  description = "AWS VPC security groups to associate with the instance."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID for the launch template"
}

variable "user_scripts" {
  description = "User provided scripts(len = instance_count) to run during the instance startup."
}

variable "private_ips" {
  description = "Custom private IP addresses to associate with the instances."
}
