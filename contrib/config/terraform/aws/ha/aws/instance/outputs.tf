output "instance_ids" {
  description = "IDs of all the instances created"
  value       = aws_instance.dgraph[*].id
}
