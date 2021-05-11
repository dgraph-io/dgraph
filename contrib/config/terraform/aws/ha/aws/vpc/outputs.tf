output "vpc_id" {
  value       = aws_vpc.dgraph.id
  description = "ID of the VPC created using the module"
}

output "subnet_id" {
  value       = aws_subnet.dgraph.id
  description = "ID of the subnet created within the VPC for dgraph"
}

output "secondary_subnet_id" {
  value       = aws_subnet.dgraph_secondary.id
  description = "ID of the secondary subnet created within the VPC for dgraph"
}

output "default_sg_id" {
  value       = aws_vpc.dgraph.default_security_group_id
  description = "Default security group ID created with the VPC."
}

output "sg_id" {
  value       = aws_security_group.dgraph_services.id
  description = "Security group ID for the auxiliary security group created for dgraph."
}

output "alb_sg_id" {
  value       = aws_security_group.dgraph_alb.id
  description = "Security group ID of the sg associated with the load balancer."
}

output "client_sg_id" {
  value       = aws_security_group.dgraph_client.id
  description = "Security group that can be used by the client to connect to the dgraph alpha instance using ALB."
}
