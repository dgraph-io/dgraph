variable "region" {
  type        = string
  default     = "us-central1"
  description = "The region to deploy the compute instance in."
}

variable "zone" {
  type        = string
  default     = "us-central1-a"
  description = "Zone to create the instance in."
}

variable "project_name" {
  type        = string
  description = "Name of the GCP project to create the instance in."
}

variable "credential_file" {
  type        = string
  description = "Credential file for the GCP account."
  default     = "account.json"
}

variable "instance_image" {
  type        = string
  default     = "ubuntu-os-cloud/ubuntu-1804-lts"
  description = "Type of GCP machine image to use for the instance."
}

variable "instance_type" {
  type        = string
  default     = "n1-standard-4"
  description = "Type of GCP instance to use."
}

variable "instance_disk_size" {
  type        = number
  default     = 50
  description = "Size of the boot disk to use with the GCP instance."
}

variable "instance_name" {
  type        = string
  default     = "dgraph-standalone"
  description = "The Name tag to set for the GCP compute Instance."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation"
  default     = "21.03.0"
}

variable "assign_public_ip" {
  type        = string
  default     = "true"
  description = "Should a public IP address be assigned to the compute instance running dgraph in standalone mode."
}
