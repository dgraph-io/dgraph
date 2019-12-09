# --------------------------------------------------------------------------------
# Setup GCP provider
# --------------------------------------------------------------------------------
provider "google" {
  credentials = file(var.credential_file)
  project     = var.project_name
  region      = var.region
}

# --------------------------------------------------------------------------------
# Dgraph instance in GCP running in standalone mode.
# --------------------------------------------------------------------------------
resource "google_compute_instance" "dgraph_standalone" {
  name         = var.instance_name
  machine_type = var.instance_type
  description  = "GCP compute instance for dgrpah in standalone mode, this instance alone hosts everything including zero, alpha and ratel."

  tags = ["dgraph", "dgraph-standalone"]

  deletion_protection = false

  boot_disk {
    auto_delete = true

    initialize_params {
      image = var.instance_image
      size  = var.instance_disk_size
    }
  }

  network_interface {
    network = "default"

    access_config {
    }
  }

  metadata = {
    type = "dgraph-standalone"
  }

  # Startup script to run for the instance. This will download the dgraph binary
  # and run it as a systemd service.
  metadata_startup_script = data.template_file.setup_template.rendered
}
