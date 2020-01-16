data "template_file" "service_template" {
  count = var.instance_count

  template = count.index == 0 ? file("${path.module}/../../templates/dgraph-zero-init.service.tmpl") : file("${path.module}/../../templates/dgraph-zero.service.tmpl")

  vars = {
    private_ip      = cidrhost(var.subnet_cidr_block, count.index + 10)
    healthy_zero_ip = local.healthy_zero_ip
    index           = count.index + 1
    replicas_count  = local.replicas_count
  }
}

data "template_file" "setup_template" {
  count = var.instance_count

  template = file("${path.module}/../../templates/setup-systemd-service.sh.tmpl")

  vars = {
    systemd_service = data.template_file.service_template[count.index].rendered
    service_name    = "dgraph-zero"
    dgraph_version  = var.dgraph_version
  }
}

data "null_data_source" "ips" {
  count = var.instance_count

  inputs = {
    private = cidrhost(var.subnet_cidr_block, count.index + 10)
  }
}
