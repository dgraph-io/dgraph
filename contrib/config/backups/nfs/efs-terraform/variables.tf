#####################################################################
# Required Variables
#####################################################################

## Required by AWS Provider
variable "region" {}

## Must Supply VPC ID or VPC Tag Name
variable "vpc_id" {
    type = string
    description = "VPC ID"
    default = ""
}

variable "vpc_name" {
    type = string
    description = "VPC Tag Name used to search for VPC ID"
    default = ""
}

#####################################################################
# Optional Variables
#####################################################################
variable "eks_cluster_name" {
    type = string
    description = "Name of EKS Cluster (specify if VPC Tag Name is different that EKS Cluster Name)"
    default = ""
}

variable "dns_name" {
    type = string
    description = "Name of Server, e.g. myfileserver"
    default = ""
}

## Specify Route53 Zone ID or DNS Domain Name used to search for Route53 Zone ID
variable "dns_domain" {
    type = string
    description = "Domain used to search for Route53 DNS Zone ID, e.g. devtest.mycompany.com"
    default = ""
}

variable "zone_id" {
    type = string
    description = "Route53 DNS Zone ID"
    default = ""
}

variable "encrypted" {
  type        = bool
  description = "If true, the file system will be encrypted"
  default     = false
}

variable "create_env_sh" {
  type        = bool
  description = "If true, env.sh will be created for use with Docker-Compose or Kubernetes"
  default     = true
}

variable "security_groups" {
  type        = list(string)
  description = "Security group IDs to allow access to the EFS"
  default = []
}


## Supply List of Subnet IDs or search for private subnets based on eksctl tag names
variable "subnets" {
  type        = list(string)
  description = "Subnet IDs"
  default = [] 
}