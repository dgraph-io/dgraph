variable "deployment_name" {
  type        = string
  description = "Name to associate with the created load balancer target group resource."
}

variable "port" {
  type        = number
  description = "Port for the load balancer target group."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID of the dgraph cluster we created."
}

variable "health_check_interval" {
  type        = number
  description = "Periodic health check interval time, defaults to 10."
  default     = 10
}

variable "timeout" {
  type        = number
  description = "Timeout for the health check corresponding to target group, defaults to 10."
  default     = 5
}

variable "health_check_path" {
  type        = string
  description = "Path for health check of the target group, defaults to /health."
  default     = "/health"
}

variable "protocol" {
  type        = string
  description = "Protocol to use for health check, defaults to HTTP."
  default     = "HTTP"
}
