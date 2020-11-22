variable "deployment_name" {
  type        = string
  description = "Name of the ASG deployment."
}

variable "instance_count" {
  type        = number
  description = "Desired instance count for the autoscaling group."
}

variable "launch_template_id" {
  type        = string
  description = "Launch configuration template ID."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID for the VPC zone"
}

variable "target_group_arn" {
  type        = string
  description = "Target group ARN to associate with the autoscaling group."
}
