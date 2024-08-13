data "template_file" "service_template" {
  template = file("${path.module}/../../templates/dgraph-alpha.service.tmpl")

  vars = {
    healthy_zero_ip = var.healthy_zero_ip
  }
}

data "template_file" "setup_template" {
  template = file("${path.module}/../../templates/setup-systemd-service.sh.tmpl")

  vars = {
    systemd_service = data.template_file.service_template.rendered
    service_name    = "dgraph-alpha"
    dgraph_version  = var.dgraph_version
  }
}
