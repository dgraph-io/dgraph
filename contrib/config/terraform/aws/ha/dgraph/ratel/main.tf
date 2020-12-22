locals {
  deployment_name = "${var.name}-ratel"
  ratel_port      = 8000
}

module "aws_tg" {
  source = "./../../aws/target_group"

  vpc_id = var.vpc_id
  port   = local.ratel_port

  deployment_name   = local.deployment_name
  health_check_path = "/"
}

module "aws_lb_listner" {
  source = "./../../aws/load_balancer/lb_listner"

  load_balancer_arn = var.lb_arn
  target_group_arn  = module.aws_tg.arn

  port = local.ratel_port
}

module "aws_instance" {
  source = "./../../aws/instance"

  deployment_name = local.deployment_name

  disk_size     = var.disk_size
  disk_iops     = var.disk_iops
  instance_type = var.instance_type
  ami_id        = var.ami_id
  key_pair_name = var.key_pair_name
  sg_id         = var.sg_id
  subnet_id     = var.subnet_id
  private_ips   = data.null_data_source.ips

  user_scripts     = data.template_file.setup_template
  instance_count   = var.instance_count
}

resource "aws_lb_target_group_attachment" "dgraph_ratel" {
  count = var.instance_count

  target_group_arn = module.aws_tg.arn
  target_id        = module.aws_instance.instance_ids[count.index]
  port             = local.ratel_port
}
