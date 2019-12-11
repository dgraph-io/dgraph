resource "aws_network_interface" "dgraph" {
  count = var.instance_count

  subnet_id   = var.subnet_id
  private_ips = [var.private_ips[count.index].outputs["private"]]
  security_groups = [var.sg_id]

  tags = {
    Name = "${var.deployment_name}-interface-${count.index}"
  }
}

resource "aws_instance" "dgraph" {
  count = var.instance_count

  ami           = var.ami_id
  instance_type = var.instance_type

  disable_api_termination = false
  key_name = var.key_pair_name

  network_interface {
    network_interface_id  = aws_network_interface.dgraph[count.index].id
    device_index          = 0
  }

  credit_specification {
    cpu_credits = "standard"
  }

  root_block_device {
    volume_size = var.disk_size
    delete_on_termination = true
  }

  user_data = base64encode(var.user_scripts[count.index].rendered)

  tags = {
    Name = var.deployment_name
  }
}
