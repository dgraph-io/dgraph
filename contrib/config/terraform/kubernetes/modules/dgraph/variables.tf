variable "kubeconfig_path" {
  type        = string
  description = "The path of the Kubeconfig file to use to provision resources on the cluster"
}

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

variable "ingress_whitelist_cidrs" {
  type        = list
  default     = ["0.0.0.0/0"]
  description = "The CIDRs whitelisted at the service load balancer"
}

variable "namespace" {
  type        = string
  default     = "dgraph"
  description = "The namespace to deploy the Dgraph pods to"
}

variable "zero_replicas" {
  type        = number
  default     = 3
  description = "The number of Zero replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Zero pod"
}

variable "zero_persistence" {
  type        = bool
  default     = true
  description = "If enabled mounts a persistent disk to the Zero pods"
}

variable "zero_storage_size_gb" {
  type        = number
  default     = 10
  description = "The size of the persistent disk to attach to the Zero pods in GiB"
}

variable "alpha_replicas" {
  type        = number
  default     = 3
  description = "The number of Alpha replicas to create. Overridden by the ha variable which when disabled leads to creation of only 1 Alpha pod"
}

variable "alpha_initialize_data" {
  type        = bool
  default     = false
  description = "If set, runs an init container to help with loading the data into Alpha"
}

variable "alpha_persistence" {
  type        = bool
  default     = true
  description = "If enabled mounts a persistent disk to the Alpha pods"
}

variable "alpha_storage_size_gb" {
  type        = number
  default     = 10
  description = "The size of the persistent disk to attach to the Alpha pods in GiB"
}

variable "alpha_lru_size_mb" {
  type        = number
  default     = 2048
  description = "The LRU cache to enable on Alpha pods in MiB"
}

variable "zero_address" {
  type        = string
  description = "The address of the Zero pod for Alpha to register against"
}
