output "vpc_id" {
  description = "ID of the VPC created for dgraph cluster."
  value       = module.aws_vpc.vpc_id
}

output "client_sg_id" {
  description = "Security group that can be used with the client."
  value       = module.aws_vpc.client_sg_id
}

output "lb_dns_name" {
  description = "DNS associated with the application load balancer created for dgraph."
  value       = module.aws_lb.dns_name
}

output "healthy_zero_ip" {
  description = "IP address of a healthy zero(initial zero) created."
  value       = module.zero.healthy_zero_ip
}

output "zero_private_ips" {
  description = "IP addresses of the created dgraph zero instances."
  value       = module.zero.private_ips
}

output "alpha_target_group_id" {
  description = "ID of the target group associated with alpha autoscaling group."
  value       = module.alpha.target_group_id
}

output "alpha_auto_scaling_group_id" {
  description = "ID of the autoscaling group created for dgrpah alpha nodes."
  value       = module.alpha.auto_scaling_group_id
}

output "alpha_port" {
  description = "HTTP port for dgraph alpha component."
  value       = module.alpha.alpha_port
}
