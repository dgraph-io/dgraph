# --------------------------------------------------------------------------------
# Setup AWS provider
# --------------------------------------------------------------------------------
provider "aws" {
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
  region     = var.region
  profile    = var.profile
}

# --------------------------------------------------------------------------------
# Security group for dgraph instance in standalone mode.
# --------------------------------------------------------------------------------
resource "aws_security_group" "dgraph_standalone" {
  name = var.instance_name

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port = var.ssh_port
    to_port   = var.ssh_port
    protocol  = "tcp"

    # To keep this setup simple, we allow incoming SSH requests from any IP.
    # In real-world usage, you should only allow SSH requests from trusted servers,
    # such as a bastion host or VPN server.
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = var.dgraph_alpha_port
    to_port     = var.dgraph_alpha_port
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

}

# --------------------------------------------------------------------------------
# Create an AWS key pair for ssh purposes.
# --------------------------------------------------------------------------------
resource "aws_key_pair" "dgraph_standalone_key" {
  key_name   = var.key_pair_name
  public_key = var.public_key
}

# --------------------------------------------------------------------------------
# Launch a dgraph standalone EC2 instance.
# --------------------------------------------------------------------------------
resource "aws_instance" "dgraph_standalone" {
  ami                         = var.aws_ami
  associate_public_ip_address = var.assign_public_ip

  monitoring                           = true
  disable_api_termination              = false
  instance_initiated_shutdown_behavior = "terminate"

  instance_type = var.instance_type
  key_name      = var.key_pair_name

  # We are not using security group ID here as this is a standalone mode
  # which deploys dgraph in a single EC2 instance without any VPC constraints.
  security_groups = [aws_security_group.dgraph_standalone.name]

  ebs_block_device {
    device_name           = "/dev/sda1"
    volume_size           = 20
    volume_type           = "standard"
    delete_on_termination = true
  }

  # base64encoded user provided script to run at the time of instance
  # initialization.
  user_data_base64 = base64encode(data.template_file.setup_template.rendered)

  tags = {
    Name = "dgraph-standalone"
  }
}
