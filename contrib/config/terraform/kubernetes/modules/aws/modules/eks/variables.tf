variable "cluster_name" {
  type        = string
  default     = "dgraph"
  description = "The name of the cluster"
}

variable "region" {
  type        = string
  default     = "ap-south-1"
  description = "AWS region to host your network"
}

variable "ha" {
  type        = bool
  default     = true
  description = "Enable or disable HA deployment of Dgraph"
}

variable "instance_types" {
  type        = list
  default     = ["m5.large"]
  description = "The list of instance types to run as worker nodes"
}

variable "worker_nodes_count" {
  type        = number
  default     = 3
  description = "The number of worker nodes to provision with the EKS cluster"
}

variable "vpc_id" {
  type        = string
  description = "The ID of the VPC to create the EKS cluster in"
}

variable "cluster_subnet_ids" {
  type        = list
  description = "The subnets earmarked to create the EKS cluster in"
}

variable "db_subnet_ids" {
  type        = list
  description = "The private subnet IDs"
}

variable "ingress_whitelist_cidrs" {
  type        = list
  default     = ["0.0.0.0/0"]
  description = "The IPs whitelisted on the load balancer"
}
