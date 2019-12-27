# --------------------------------------------------------------------------------
# AWS Launch template for configuring EC2 instances.
# --------------------------------------------------------------------------------
resource "aws_launch_template" "dgraph" {
  name        = var.deployment_name
  description = "Launch template for dgraph(${var.deployment_name}) instances"

  block_device_mappings {
    device_name = "/dev/sda1"

    ebs {
      volume_size           = var.disk_size
      volume_type           = "io1"
      iops                  = var.disk_iops
      delete_on_termination = false
    }
  }

  capacity_reservation_specification {
    capacity_reservation_preference = "open"
  }

  credit_specification {
    cpu_credits = "standard"
  }

  disable_api_termination = false
  # ebs_optimized           = true

  image_id = var.ami_id

  instance_initiated_shutdown_behavior = "terminate"

  instance_type = var.instance_type
  key_name      = var.key_pair_name

  monitoring {
    enabled = true
  }

  network_interfaces {
    delete_on_termination       = true
    associate_public_ip_address = false
    subnet_id                   = var.subnet_id

    security_groups = [var.vpc_sg_id]
  }

  tag_specifications {
    resource_type = "instance"

    tags = {
      Name = var.deployment_name
    }
  }

  user_data = var.user_script
}
