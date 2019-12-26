variable "region" {
    default = "us-east-2"
    description = "The region of the ec2 instance"
}

variable "shared_credentials_file" {
    default = "./creds/aws_secrets"
    description = "Path to aws shared credentials secret file"
}

variable "profile" {
    default = "terraform"
}

variable "ami" {
    default = "ami-0c55b159cbfafe1f0"
    description = "Type of Amazon machine image"
}

variable "key_pair_name" {
  default = "aws"
  description = "The EC2 Key Pair to associate with the EC2 Instance for SSH access."
}

variable "instance_type" {
    default = "t2.micro"
    description="EC2 instance resource type"
}

variable "instance_name" {
  default = "dgraph"
  description = "The Name tag to set for the EC2 Instance."
}

variable "ssh_port" {
  default = 22
  description = "The port the EC2 Instance should listen on for SSH requests."
}

variable "ssh_user" {
  description = "SSH user name to use for remote exec connections,"
  default     = "ubuntu"
}

variable "dgraph_version" {
    description = "Dgraph version for installation"
    default = "1.0.13"   
}

variable "dgraph_ui_port" {
    description = "Port number of ratel interface"
    default="8000"
}
