variable "prefix" {
  type        = string
  default     = "dgraph"
  description = "The namespace prefix for all resources"
}

variable "cidr" {
  type        = string
  default     = "10.20.0.0/16"
  description = "The CIDR of the VPC"
}

variable "region" {
  type        = string
  default     = "ap-south-1"
  description = "The region to deploy the resources in"
}

variable "ha" {
  type        = bool
  default     = true
  description = "Enable or disable HA deployment of Dgraph"
}

variable "ingress_whitelist_cidrs" {
  type        = list
  default     = ["0.0.0.0/0"]
  description = "The CIDRs whitelisted at the service load balancer"
}

variable "worker_nodes_count" {
  type        = number
  default     = 3
  description = "The number of worker nodes to provision with the EKS cluster"
}

variable "instance_types" {
  type        = list
  default     = ["m5.large"]
  description = "The list of instance types to run as worker nodes"
}
