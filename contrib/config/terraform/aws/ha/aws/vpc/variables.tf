variable "name" {
  type        = string
  description = "Name tag to apply to AWS VPC we are creating for dgraph"
}

variable "cidr_block" {
  type        = string
  description = "CIDR block to associate with the VPC."
}

variable "subnet_cidr_block" {
  type        = string
  description = "CIDR block for the subnet."
}

variable "secondary_subnet_cidr_block" {
  type        = string
  description = "Secondary CIDR block for the subnet to create within the VPC, this subnet will be used for dgraph deployment."
}
