output "dns_name" {
  description = "DNS name of the load balancer created."
  value       = aws_lb.dgraph.dns_name
}

output "id" {
  description = "ID of the created load balancer resource."
  value       = aws_lb.dgraph.id
}

output "arn" {
  description = "ARN of the created load balancer resource."
  value       = aws_lb.dgraph.arn
}
