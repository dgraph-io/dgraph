output "arn" {
  description = "ARN of the target group created."
  value       = aws_lb_target_group.dgraph.arn
}
