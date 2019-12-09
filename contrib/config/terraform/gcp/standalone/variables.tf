variable "region" {
  type        = string
  default     = "us-east-2"
  description = "The region to deploy the compute instance in."
}

variable "project_name" {
  type    = string
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
  default     = "1.1.0"   
}

variable "dgraph_ui_port" {
  type        = string
  description = "Port number of ratel interface"
  default     = "8000"
}
