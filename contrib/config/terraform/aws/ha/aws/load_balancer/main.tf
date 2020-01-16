resource "aws_lb" "dgraph" {
  name               = var.deployment_name

  internal           = false
  load_balancer_type = "application"

  security_groups    = [var.sg_id]
  subnets            = [var.subnet_id, var.secondary_subnet_id]

  enable_deletion_protection = false
  enable_http2               = true

  # access_logs {
  #   bucket  = "${aws_s3_bucket.lb_logs.bucket}"
  #   prefix  = "test-lb"
  #   enabled = true
  # }

  tags = {
    Name = var.deployment_name
  }
}
