output "alpha_completed" {
  value = true
}

output "target_group_id" {
  description = "ID of the target group associated with alpha autoscaling group."
  value       = module.aws_tg.id
}

output "auto_scaling_group_id" {
  description = "ID of the autoscaling group created for dgrpah alpha nodes."
  value       = module.aws_asg.id
}

output "alpha_port" {
  description = "HTTP port for dgraph alpha component."
  value       = local.alpha_port
}
