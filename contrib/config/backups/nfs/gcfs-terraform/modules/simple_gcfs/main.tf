# gcloud filestore instances create ${MY_FS_NAME} \
#   --project=${MY_PROJECT} \
#   --zone=${MY_ZONE} \
#   --tier=${MY_FS_TIER} \
#   --file-share=name="${MY_FS_SHARE_NAME}",capacity=${MY_FS_CAPACITY} \
#   --network=name="${MY_NETWORK_NAME}"


# MY_PROJECT=${MY_PROJECT:-$(gcloud config get-value project)}
# MY_ZONE=${MY_ZONE:-"us-central1-b"}
# MY_FS_TIER=${MY_FS_TIER:-"STANDARD"}
# MY_FS_CAPACITY=${MY_FS_CAPACITY:-"1TB"}
# MY_FS_SHARE_NAME=${MY_FS_SHARE_NAME:-"volumes"}
# MY_NETWORK_NAME=${MY_NETWORK_NAME:-"default"}
# MY_FS_NAME=${MY_FS_NAME:-$1}
# CREATE_ENV_VALUES=${CREATE_ENV_VALUES:-"true"}

variable "name" {}
variable "zone" { default = "us-central1-b" }
variable "tier" { default = "STANDARD" }
variable "network" { default = "default" }
variable "capacity_gb" { default = 1024 }
variable "share_name" { default = "volumes" }

resource "google_filestore_instance" "instance" {
  name = var.name
  zone = var.zone
  tier = var.tier

  file_shares {
    capacity_gb = var.capacity_gb
    name        = var.share_name
  }

  networks {
    network = var.network
    modes   = ["MODE_IPV4"]
  }
}

output "nfs_server" {
  value = google_filestore_instance.instance.networks[0].ip_addresses[0]
}

output "nfs_path" {
  value = "/${google_filestore_instance.instance.file_shares[0].name}"
}
