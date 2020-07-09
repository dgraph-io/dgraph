provider "aws" {
  version = ">= 2.57.0"
  region  = var.region
}

provider "random" {
  version = "~> 2.3.0"
}

provider "local" {
  version = "~> 1.4.0"
}

provider "null" {
  version = "~> 2.1.2"
}

provider "template" {
  version = "~> 2.1.2"
}
