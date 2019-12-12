output "healthy_zero_ip" {
  description = "IP address of a healthy zero created by the module."
  value       = local.healthy_zero_ip
}

output "private_ips" {
  description = "IP addresses of the created dgraph zero instances."
  value       = [
    for ip_obj in data.null_data_source.ips:
    ip_obj.outputs.private
  ]
}
