#####################################################################
# Variables
#####################################################################
variable "service_account_id" {}
variable "display_name" {}
variable "project_id" {}
variable "roles" {
  description = "IAM roles to be added to the service account. See https://cloud.google.com/iam/docs/understanding-roles."
  type        = list(string)
  default     = []
}

#####################################################################
# Locals
#####################################################################
locals {
  roles           = toset(var.roles)
  sensitive_roles = ["roles/owner"]
  filtered_roles  = setsubtract(local.roles, local.sensitive_roles)
}

#####################################################################
# Resources
#####################################################################
resource "google_service_account" "service_account" {
  account_id   = var.service_account_id
  display_name = var.display_name
  project      = var.project_id
}

resource "google_service_account_key" "key" {
  service_account_id = google_service_account.service_account.name
}

resource "google_project_iam_member" "project_roles" {
  for_each = local.filtered_roles

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.service_account.email}"
}

#####################################################################
# Output
#####################################################################
output "key" {
  description = "Service account key (for single use)."
  value       = base64decode(google_service_account_key.key.private_key)
}

output "email" {
  description = "The fully qualified email address of the created service account."
  value       = google_service_account.service_account.email
}
