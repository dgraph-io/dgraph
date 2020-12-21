output "vpc_id" {
  value = aws_vpc.vpc.id
}

output "db_subnet_ids" {
  value = aws_subnet.db_subnets.*.id
}

output "cluster_subnet_ids" {
  value = concat(aws_subnet.db_subnets.*.id, aws_subnet.lb_subnets.*.id)
}
