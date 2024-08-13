
output "efs_arn" {
  value       = module.efs.arn
  description = "EFS ARN"
}

output "efs_id" {
  value       = module.efs.id
  description = "EFS ID"
}

output "efs_host" {
  value       = module.efs.host
  description = "Route53 DNS hostname for the EFS"
}

output "efs_dns_name" {
  value       = module.efs.dns_name
  description = "EFS DNS name"
}

output "efs_mount_target_dns_names" {
  value       = module.efs.mount_target_dns_names
  description = "List of EFS mount target DNS names"
}

output "efs_mount_target_ids" {
  value       = module.efs.mount_target_ids
  description = "List of EFS mount target IDs (one per Availability Zone)"
}

output "efs_mount_target_ips" {
  value       = module.efs.mount_target_ips
  description = "List of EFS mount target IPs (one per Availability Zone)"
}

output "efs_network_interface_ids" {
  value       = module.efs.network_interface_ids
  description = "List of mount target network interface IDs"
}

output "security_group_id" {
  value       = module.efs.security_group_id
  description = "EFS Security Group ID"
}

output "security_group_arn" {
  value       = module.efs.security_group_arn
  description = "EFS Security Group ARN"
}

output "security_group_name" {
  value       = module.efs.security_group_name
  description = "EFS Security Group name"
}
