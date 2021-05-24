output "zero_public_dns" {
  value       = module.zero.zero_public_dns
  description = "The hostname of the Zero service LB"
}

output "alpha_public_dns" {
  value       = module.alpha.alpha_public_dns
  description = "The hostname of the Alpha service LB"
}

output "alpha_indexzero_public_dns" {
  value       = module.alpha.alpha_indexzero_public_dns
  description = "The hostname of the Alpha[0] pod"
}
