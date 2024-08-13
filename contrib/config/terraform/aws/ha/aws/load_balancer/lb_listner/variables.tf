variable "load_balancer_arn" {
  type        = string
  description = "ARN of the load balancer to attach the listner to."
}

variable "target_group_arn" {
  type        = string
  description = "ARN of the target group to forward the request on for the listner rule."
}

variable "port" {
  type        = string
  description = "Port the listner listen to in the load balancer."
}

variable "protocol" {
  type        = string
  description = "Protocol for the listner to respond to, defaults to HTTP."
  default     = "HTTP"
}
