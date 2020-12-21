resource "aws_autoscaling_group" "dgraph" {
  name                      = var.deployment_name

  max_size                  = var.instance_count + 1
  min_size                  = var.instance_count - 1
  desired_capacity          = var.instance_count

  vpc_zone_identifier       = [var.subnet_id]

  launch_template {
    id      = var.launch_template_id
    version = "$Latest"
  }

  tag {
    key                 = "name"
    value               = var.deployment_name
    propagate_at_launch = true
  }

  timeouts {
    delete = "15m"
  }

  target_group_arns = [var.target_group_arn]
}
