locals {
  ratel_service = file("${path.module}/../../templates/dgraph-ratel.service.tmpl")
}

data "template_file" "setup_template" {
  count = var.instance_count
  template = file("${path.module}/../../templates/setup-systemd-service.sh.tmpl")

  vars = {
    systemd_service = local.ratel_service
    service_name    = "dgraph-ratel"
    dgraph_version  = var.dgraph_version
  }
}

data "null_data_source" "ips" {
  count = var.instance_count
  
  inputs = {
    private = cidrhost(var.subnet_cidr_block, count.index + 9)
  }
}
