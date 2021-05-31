output "zero_public_dns" {
  value       = module.dgraph.zero_public_dns
  description = "The hostname of the Zero service LB"
}

output "alpha_public_dns" {
  value       = module.dgraph.alpha_public_dns
  description = "The hostname of the Alpha service LB"
}

output "alpha_indexzero_public_dns" {
  value       = module.dgraph.alpha_indexzero_public_dns
  description = "The hostname of the Alpha[0] service LB"
}
