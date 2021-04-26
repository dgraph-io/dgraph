variable "region" {
  type        = string
  default     = "us-east-2"
  description = "The region to deploy the EC2 instance in."
}

variable "profile" {
  type    = string
  default = "terraform"
}

variable "aws_access_key" {
  type        = string
  description = "Access key for the AWS account to create the dgraph deployment in."
}

variable "aws_secret_key" {
  type        = string
  description = "Secret key for the AWS account."
}

variable "aws_ami" {
  type        = string
  default     = "ami-0c55b159cbfafe1f0"
  description = "Type of Amazon machine image to use for the instance."
}

variable "key_pair_name" {
  type        = string
  default     = "dgraph-standalone-key"
  description = "The EC2 Key Pair to associate with the EC2 Instance for SSH access."
}

variable "public_key" {
  type        = string
  description = "Public SSH key to be associated with the instance."
}

variable "ssh_port" {
  type        = number
  default     = 22
  description = "The port the EC2 Instance should listen on for SSH requests."
}

variable "instance_type" {
  type        = string
  default     = "t2.micro"
  description = "EC2 instance resource type"
}

variable "instance_name" {
  type        = string
  default     = "dgraph-standalone"
  description = "The Name tag to set for the EC2 Instance."
}

variable "dgraph_version" {
  type        = string
  description = "Dgraph version for installation"
  default     = "21.03.0"
}

variable "dgraph_alpha_port" {
  type        = string
  description = "Port number of dgraph alpha to connect to."
  default     = "8080"
}

variable "assign_public_ip" {
  type        = string
  default     = true
  description = "Should a public IP address be assigned to the EC2 instance running dgraph in standalone mode."
}
