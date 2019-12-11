output "lb_dns_name" {
    description = "DNS associated with the application load balancer created for dgraph."
    value       = module.aws_lb.dns_name
}