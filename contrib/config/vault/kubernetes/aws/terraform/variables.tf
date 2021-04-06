variable "region" {}

variable "create_env_sh" {
  type        = bool
  default     = true
  description = "Toogles whether env.sh is created"
}

variable "user_enabled" {
  type        = bool
  default     = true
  description = "Toggles whether resource is created"
}

variable "access_key_enabled" {
  type        = bool
  description = "Set to true to have the access key get created"
  default     = true
}

variable "path" {
  type        = string
  description = "Path user is created"
  default     = "/"
}

variable "force_destroy" {
  type        = bool
  description = "User destroy even if it has non-terraform managed access keys, login profile, or MFA device"
  default     = false
}
