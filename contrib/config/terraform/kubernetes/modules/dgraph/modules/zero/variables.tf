variable "prefix" {
  type        = string
  default     = "dgraph"
  description = "The namespace prefix for all resources"
}

variable "ha" {
  type        = bool
  default     = true
  description = "Enable or disable HA deployment of Dgraph"
}

variable "namespace" {
  type        = string
  default     = "dgraph"
  description = "The namespace to deploy the Dgraph pods to"
}

variable "persistence" {
  type        = bool
  default     = true
  description = "If set to true, mounts a persistent disk to the Zero pods"
}

variable "storage_size_gb" {
  type        = number
  default     = 10
  description = "The size of the persistent disk to attach to the Zero pods in GiB"
}

variable "replicas" {
  type        = number
  default     = 3
  description = "The number of Zero replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Zero pod"
}

variable "ingress_whitelist_cidrs" {
  type        = list
  default     = ["0.0.0.0/0"]
  description = "The IPs whitelisted on the load balancer"
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

variable "http_port" {
  type        = number
  default     = 6080
  description = "The HTTP port exposed by the Dgraph Zero service"
}

variable "grpc_port" {
  type        = number
  default     = 5080
  description = "the gRPC port exposed by the Dgraph Zero service"
}

variable "command" {
  type        = list
  default     = ["bash", "/cmd/dgraph_zero.sh"]
  description = "The command to run to start the Dgraph Zero service"
}

variable "persistence_mount_path" {
  type    = string
  default = "/dgraph"
}

# Probes
variable "liveness_probe_path" {
  type    = string
  default = "/health"
}

variable "liveness_probe_initial_delay_seconds" {
  type    = number
  default = 15
}

variable "liveness_probe_timeout_seconds" {
  type    = number
  default = 5
}

variable "liveness_period_seconds" {
  type    = number
  default = 10
}

variable "liveness_probe_success_threshold" {
  type    = number
  default = 1
}

variable "liveness_probe_failure_threshold" {
  type    = number
  default = 6
}

variable "readiness_probe_path" {
  type    = string
  default = "/state"
}

variable "readiness_probe_initial_delay_seconds" {
  type    = number
  default = 15
}

variable "readiness_probe_timeout_seconds" {
  type    = number
  default = 5
}

variable "readiness_period_seconds" {
  type    = number
  default = 10
}

variable "readiness_probe_success_threshold" {
  type    = number
  default = 1
}

variable "readiness_probe_failure_threshold" {
  type    = number
  default = 6
}


variable "termination_grace_period_seconds" {
  type    = number
  default = 60
}

variable "namespace_resource" {
  type    = any
  default = null
}
