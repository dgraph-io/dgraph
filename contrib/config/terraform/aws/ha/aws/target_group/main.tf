resource "aws_lb_target_group" "dgraph" {
  name     = var.deployment_name
  port     = var.port
  protocol = var.protocol
  vpc_id   = var.vpc_id

  health_check {
    enabled  = true
    interval = var.health_check_interval
    path     = var.health_check_path
    port     = var.port
    timeout  = var.timeout

    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}
