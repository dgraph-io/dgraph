variable "deployment_name" {
  type        = string
  description = "Name to associate with the launch template configuration"
}

variable "disk_size" {
  type        = string
  description = "Disk size to associate with the instance running through the launch template."
}

variable "disk_iops" {
  type        = number
  description = "IOPS limit for the disk associated with the instance."
}

variable "instance_type" {
  type        = string
  description = "Type of instance to launch from the launch template."
}

variable "ami_id" {
  type        = string
  description = "AMI to launch the instance with."
}

variable "key_pair_name" {
  type        = string
  description = "AWS key-pair name to associate with the launched instance for SSH access"
}

variable "vpc_sg_id" {
  type        = string
  description = "AWS VPC security groups to associate with the instance."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID for the launch template"
}

variable "user_script" {
  type        = string
  description = "User provided script to run during the instance startup."
}
