variable "cluster_name" {
  type        = string
  default     = "dgraph"
  description = "The name of the VPC/EKS cluster. Will be used to prefix the applications"
}

variable "region" {
  type        = string
  description = "AWS region to host your network"
}

variable "cidr" {
  type        = string
  description = "The CIDR of the VPC"
}

variable "ingress_ports" {
  type        = list
  default     = [5080, 6080, 8000, 8080, 9080]
  description = "The ingress ports opened at the LB NACL. The specific IP whitelists are applied at the security group level"
}

