output "lb_dns_name" {
  description = "DNS associated with the application load balancer created for dgraph."
  value       = module.dgraph.lb_dns_name
}

output "vpc_id" {
  description = "ID of the VPC created for dgraph cluster."
  value       = module.dgraph.vpc_id
}

output "client_sg_id" {
  description = "Security group that can be used with the client."
  value       = module.dgraph.client_sg_id
}

output "healthy_zero_ip" {
  description = "IP address of a healthy zero(initial zero) created."
  value       = module.dgraph.healthy_zero_ip
}

output "zero_private_ips" {
  description = "IP addresses of the created dgraph zero instances."
  value       = module.dgraph.zero_private_ips
}

output "alpha_target_group_id" {
  description = "ID of the target group associated with alpha autoscaling group."
  value       = module.dgraph.alpha_target_group_id
}

output "alpha_auto_scaling_group_id" {
  description = "ID of the autoscaling group created for dgraph alpha nodes."
  value       = module.dgraph.alpha_auto_scaling_group_id
}

output "alpha_port" {
  description = "HTTP port for dgraph alpha component."
  value       = module.dgraph.alpha_port
}
