locals {
  deployment_name = "${var.name}-zero"
  healthy_zero_ip = cidrhost(var.subnet_cidr_block, 10)
  replicas_count  = var.instance_count > 3 ? 3 : var.instance_count
}

module "aws_instance" {
  source = "./../../aws/instance"

  instance_count = var.instance_count

  deployment_name = local.deployment_name

  disk_size     = var.disk_size
  disk_iops     = var.disk_iops
  instance_type = var.instance_type
  ami_id        = var.ami_id
  key_pair_name = var.key_pair_name
  sg_id         = var.sg_id
  subnet_id     = var.subnet_id
  private_ips   = data.null_data_source.ips
  user_scripts  = data.template_file.setup_template
}
