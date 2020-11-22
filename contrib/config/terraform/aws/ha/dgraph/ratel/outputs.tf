output "target_group_id" {
  description = "Target group associated with the created ratel instances."
  value       = module.aws_tg.id
}

output "private_ips" {
  description = "IP addresses of the created dgraph ratel instances."
  value       = [
    for ip_obj in data.null_data_source.ips:
    ip_obj.outputs.private
  ]
}

output "ratel_port" {
  description = "Port to which ratel UI server is listening."
  value       = local.ratel_port
}
