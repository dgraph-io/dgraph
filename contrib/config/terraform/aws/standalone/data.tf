# --------------------------------------------------------------------------------
# Setup template script for dgraph in standalone mode
# --------------------------------------------------------------------------------
data "template_file" "setup_template" {
  template = file("${path.module}/templates/setup.tmpl")

  # Systemd service description for dgraph components.
  vars = {
    dgraph_zero_service = "${file("${path.module}/templates/dgraph-zero.service")}"
    dgraph_service      = "${file("${path.module}/templates/dgraph.service")}"
    dgraph_version      = "${var.dgraph_version}"
  }
}
