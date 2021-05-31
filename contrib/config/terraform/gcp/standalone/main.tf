# --------------------------------------------------------------------------------
# Setup GCP provider
# --------------------------------------------------------------------------------
provider "google" {
  credentials = file(var.credential_file)
  project     = var.project_name
  region      = var.region
  zone        = var.zone
}

# --------------------------------------------------------------------------------
# Dgraph instance in GCP running in standalone mode.
# --------------------------------------------------------------------------------
resource "google_compute_instance" "dgraph_standalone" {
  name         = var.instance_name
  machine_type = var.instance_type
  description  = "GCP compute instance for dgraph in standalone mode, this instance alone hosts everything (zero and alpha)."

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

    dynamic "access_config" {
      for_each = var.assign_public_ip == "false" ? [] : ["STANDARD"]
      content {
        network_tier = access_config.value
      }
    }
  }

  metadata = {
    type = "dgraph-standalone"
  }

  # Startup script to run for the instance. This will download the dgraph binary
  # and run it as a systemd service.
  metadata_startup_script = data.template_file.setup_template.rendered
}
