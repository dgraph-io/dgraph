packer {
  required_plugins {
    amazon = {
      version = ">= 0.0.2"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

variable "instance_type" {
  type    = string
  default = "t2.medium"
  description = "Instance type to use for creating the image"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "aws_profile" {
  type    = string
  default = "default"
}

source "amazon-ebs" "ubuntu" {
  ami_name      = "gh-runner-linux-aws-v4"
  profile       = var.aws_profile
  instance_type = var.instance_type
  region        = var.region
  source_ami_filter {
    filters = {
      name                = "ubuntu/images/*ubuntu-focal-20.04-amd64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"]
  }
  ssh_username = "ubuntu"
}

build {
  name = "gh-runner"
  sources = [
    "source.amazon-ebs.ubuntu"
  ]
  provisioner "shell" {
    inline = ["sleep 10"]
  }
  provisioner "shell" {
    script = "install-deps.sh"
  }
  provisioner "file" {
    source      = "init-runner.sh"
    destination = "/home/ubuntu/init-runner.sh"
  }
}
