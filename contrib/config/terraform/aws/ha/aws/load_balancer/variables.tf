variable "deployment_name" {
  type        = string
  description = "Name to associate with the created load balancer resource."
}

variable "sg_id" {
  type        = string
  description = "Security group to associate with the load balancer."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID for the load balancer."
}

variable "secondary_subnet_id" {
  type        = string
  description = "Secondary subnet ID for the load balancer, this must be in the different zone than subnet_id."
}
