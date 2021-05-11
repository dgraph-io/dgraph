variable "name" {
  type        = string
  description = "Name of the dgraph deployment"
}

variable "alpha_count" {
  type        = number
  description = "Number of dgraph alphas to run in the cluster, defaults to 3."
}

variable "zero_count" {
  type        = number
  description = "Number of dgraph zeros to run in the cluster, defaults to 3."
}

variable "alpha_instance_type" {
  type        = string
  description = "EC2 Instance type for dgraph alpha component."
}

variable "zero_instance_type" {
  type        = string
  description = "EC2 instance type for dgraph zero component."
}

variable "alpha_disk_size" {
  type        = string
  description = "Disk size for dgraph alpha node."
}

variable "zero_disk_size" {
  type        = string
  description = "Disk size for dgraph zero node."
}

variable "disk_iops" {
  type        = number
  description = "IOPS limit for the disk associated with the instance."
}

variable "cidr_block" {
  type        = string
  description = "CIDR block to assign to the VPC running the dgraph cluster, only used if a new VPC is created."
}

variable "subnet_cidr_block" {
  type        = string
  description = "CIDR block to create the subnet with in the VPC."
}

variable "secondary_subnet_cidr_block" {
  type        = string
  description = "Secondary CIDR block for the subnet to create within the VPC, this subnet will be used for dgraph deployment."
}

variable "ami_id" {
  type        = string
  description = "AMI to use for the instances"
}

variable "key_pair_name" {
  type        = string
  description = "Key Pair to create for the instances."
}

variable "public_key" {
  type        = string
  description = "Public key corresponding to the key pair."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation."
}
