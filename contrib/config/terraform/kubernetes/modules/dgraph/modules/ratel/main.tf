resource "kubernetes_service" "dgraph_ratel_public" {
  metadata {
    name      = "${var.prefix}-dgraph-ratel-public"
    namespace = var.namespace

    labels = {
      app = "${var.prefix}-dgraph-ratel"
    }
  }

  spec {
    port {
      name        = "ratel-http"
      port        = var.http_port
      target_port = var.http_port
    }

    selector = {
      app = "${var.prefix}-dgraph-ratel"
    }

    type = "LoadBalancer"

    load_balancer_source_ranges = var.ingress_whitelist_cidrs
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_deployment" "dgraph_ratel" {
  metadata {
    name      = "${var.prefix}-dgraph-ratel"
    namespace = var.namespace
    labels = {
      "app" = "dgraph-ratel"
    }
  }

  spec {
    selector {
      match_labels = {
        app = "${var.prefix}-dgraph-ratel"
      }
    }

    template {
      metadata {
        labels = {
          app = "${var.prefix}-dgraph-ratel"
        }
      }

      spec {
        container {
          name = "ratel"

          image             = "${var.image}:${var.image_version}"
          image_pull_policy = var.image_pull_policy
          command           = var.command

          port {
            name           = "dgraph-ratel"
            container_port = var.http_port
          }
        }
      }
    }
  }

  depends_on = [var.namespace_resource]
}
