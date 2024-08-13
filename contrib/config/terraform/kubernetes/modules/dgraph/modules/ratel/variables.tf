variable "prefix" {
  type        = string
  default     = "dgraph"
  description = "The namespace prefix for all resources"
}

variable "namespace" {
  type        = string
  default     = "dgraph"
  description = "The namespace to deploy the Dgraph pods to"
}

variable "http_port" {
  type        = number
  default     = 8000
  description = "The HTTP port exposed by Dgraph Ratel"
}

variable "image" {
  type        = string
  default     = "dgraph/dgraph"
  description = "The docker image to use to run Dgraph services"
}

variable "image_version" {
  type        = string
  default     = "latest"
  description = "The version of the Dgraph docker image to run"
}

variable "image_pull_policy" {
  type        = string
  default     = "IfNotPresent"
  description = "The default image pull policy to use while scheduling and running pods"
}

variable command {
  type        = list
  default     = ["dgraph-ratel"]
  description = "The command to run to start the Dgraph Ratel service"
}

variable "ingress_whitelist_cidrs" {
  type        = list
  default     = ["0.0.0.0/0"]
  description = "The IPs whitelisted on the service load balancer"
}

variable "namespace_resource" {
  type    = any
  default = null
}
