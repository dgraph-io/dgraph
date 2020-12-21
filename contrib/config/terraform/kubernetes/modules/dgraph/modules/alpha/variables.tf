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

variable "initialize_data" {
  type        = bool
  default     = false
  description = "If set, runs an init container to help with loading the data into Alpha"
}

variable "zero_address" {
  type        = string
  description = "The address of a Zero pod"
}

variable "persistence" {
  type        = bool
  default     = true
  description = "If set to true, mounts a persistent disk to the Alpha pods"
}

variable "storage_size_gb" {
  type        = number
  default     = 10
  description = "The size of the persistent disk to attach to the Alpha pods in GiB"
}

variable "replicas" {
  type        = number
  default     = 3
  description = "The number of Alpha replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Alpha pod"
}

variable "lru_size_mb" {
  type        = number
  default     = 1025
  description = "The LRU cache size in MiB"
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
  default     = 8080
  description = "The HTTP port of Dgraph Alpha"
}

variable "grpc_port" {
  type        = number
  default     = 9080
  description = "The external gRPC port exposed by Dgraph Alpha"
}

variable "grpc_int_port" {
  type        = number
  default     = 7080
  description = "The internal gRPC port exposed Dgraph Alpha "
}

variable "command" {
  type        = list
  default     = ["bash", "/cmd/dgraph_alpha.sh"]
  description = "The command to run to start the Dgraph Alpha service"
}

variable "init_command" {
  type        = list
  default     = ["bash", "/cmd/dgraph_init_alpha.sh"]
  description = "The command to run to start the Dgraph Alpha init container"
}

variable "persistence_mount_path" {
  type    = string
  default = "/dgraph"
}

# Probes
variable "liveness_probe_path" {
  type    = string
  default = "/health?live=1"
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
  default = "/health"
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
  type        = number
  default     = 600
  description = "The grace period to wait before sending SIGTERM to the Alpha pods"
}

variable "namespace_resource" {
  type    = any
  default = null
}
