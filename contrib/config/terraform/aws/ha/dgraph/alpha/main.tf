locals {
  deployment_name = "${var.name}-alpha"
  alpha_port      = 8080
}

module "aws_lt" {
  source = "./../../aws/launch_template"

  deployment_name = local.deployment_name
  disk_size       = var.disk_size
  disk_iops       = var.disk_iops
  instance_type   = var.instance_type

  ami_id    = var.ami_id
  vpc_sg_id = var.sg_id
  subnet_id = var.subnet_id

  key_pair_name = var.key_pair_name
  user_script   = base64encode(data.template_file.setup_template.rendered)
}

module "aws_tg" {
  source = "./../../aws/target_group"

  vpc_id = var.vpc_id
  port   = local.alpha_port

  deployment_name = local.deployment_name
}

module "aws_lb_listner" {
  source = "./../../aws/load_balancer/lb_listner"

  load_balancer_arn = var.lb_arn
  target_group_arn  = module.aws_tg.arn

  port = local.alpha_port
}

module "aws_asg" {
  source = "./../../aws/auto_scaling_group"

  deployment_name    = local.deployment_name
  launch_template_id = module.aws_lt.id
  subnet_id          = var.subnet_id
  instance_count     = var.instance_count
  target_group_arn   = module.aws_tg.arn
}
