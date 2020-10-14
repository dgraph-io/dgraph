provider "google" {
  version = "~> 3.38.0"
  region  = var.region
  project = var.project_id
}

provider "random" {
  version = "2.3.0"
}
