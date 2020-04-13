locals {
  zero_replicas         = var.ha ? var.replicas : 1
  has_pod_anti_affinity = var.ha ? [1] : []
  has_persistence       = var.persistence ? [1] : []
}

resource "kubernetes_service" "dgraph_zero_public" {
  metadata {
    name      = "${var.prefix}-dgraph-zero-public"
    namespace = var.namespace

    labels = {
      app     = "${var.prefix}-dgraph-zero"
      monitor = "zero-dgraph-io"
    }
  }

  spec {
    port {
      name        = "zero-http"
      port        = var.http_port
      target_port = var.http_port
    }

    port {
      name        = "zero-grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
    }

    selector = {
      app = "${var.prefix}-dgraph-zero"
    }

    type = "LoadBalancer"

    load_balancer_source_ranges = var.ingress_whitelist_cidrs
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_service" "dgraph_zero" {
  metadata {
    name      = "${var.prefix}-dgraph-zero"
    namespace = var.namespace

    labels = {
      app = "${var.prefix}-dgraph-zero"
    }
  }

  spec {
    port {
      name        = "zero-grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
    }

    selector = {
      app = "${var.prefix}-dgraph-zero"
    }

    cluster_ip                  = "None"
    type                        = "ClusterIP"
    publish_not_ready_addresses = true
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_config_map" "dgraph_zero_command" {
  metadata {
    name      = "${var.prefix}-dgraph-zero-cm"
    namespace = var.namespace
  }

  data = {
    "dgraph_zero.sh" = var.ha ? templatefile("${path.module}/templates/zero-ha.tpl", { prefix = var.prefix, namespace = var.namespace, replicas = local.zero_replicas }) : templatefile("${path.module}/templates/zero.tpl", {})
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_stateful_set" "dgraph_zero" {
  metadata {
    name      = "${var.prefix}-dgraph-zero"
    namespace = var.namespace
  }

  spec {
    update_strategy {
      type = "RollingUpdate"
    }

    dynamic "volume_claim_template" {
      for_each = local.has_persistence
      content {
        metadata {
          name      = "datadir"
          namespace = var.namespace

          annotations = { "volume.alpha.kubernetes.io/storage-class" = "anything" }
        }

        spec {
          access_modes = ["ReadWriteOnce"]

          resources {
            requests = {
              storage = "${var.storage_size_gb}Gi"
            }
          }
        }
      }
    }

    service_name = "${var.prefix}-dgraph-zero"

    replicas = local.zero_replicas

    selector {
      match_labels = {
        app = "${var.prefix}-dgraph-zero"
      }
    }

    template {
      metadata {
        labels = {
          app = "${var.prefix}-dgraph-zero"
        }
      }

      spec {
        dynamic "affinity" {
          for_each = local.has_pod_anti_affinity
          content {
            pod_anti_affinity {
              preferred_during_scheduling_ignored_during_execution {
                weight = 100
                pod_affinity_term {
                  label_selector {
                    match_expressions {
                      key      = "kubernetes.io/hostname"
                      operator = "In"
                      values   = ["${var.prefix}-dgraph-zero"]
                    }
                  }
                  topology_key = "kubernetes.io/hostname"
                }
              }
            }
          }
        }

        termination_grace_period_seconds = 60

        volume {
          name = "dgraph-zero-command"

          config_map {
            name         = "${var.prefix}-dgraph-zero-cm"
            default_mode = "0755"
          }
        }

        container {
          name = "zero"

          image             = "${var.image}:${var.image_version}"
          image_pull_policy = var.image_pull_policy

          env {
            name  = "POD_NAMESPACE"
            value = var.namespace
          }

          command = var.command

          port {
            name           = "zero-grpc"
            container_port = var.grpc_port
          }

          port {
            name           = "zero-http"
            container_port = var.http_port
          }

          dynamic "volume_mount" {
            for_each = local.has_persistence
            content {
              name       = "datadir"
              mount_path = var.persistence_mount_path
            }
          }

          volume_mount {
            name       = "dgraph-zero-command"
            mount_path = "/cmd"
          }

          liveness_probe {
            http_get {
              path = var.liveness_probe_path
              port = var.http_port
            }

            initial_delay_seconds = var.liveness_probe_initial_delay_seconds
            timeout_seconds       = var.liveness_probe_timeout_seconds
            period_seconds        = var.liveness_period_seconds
            success_threshold     = var.liveness_probe_success_threshold
            failure_threshold     = var.liveness_probe_failure_threshold
          }

          readiness_probe {
            http_get {
              path = var.readiness_probe_path
              port = var.http_port
            }

            initial_delay_seconds = var.readiness_probe_initial_delay_seconds
            timeout_seconds       = var.readiness_probe_timeout_seconds
            period_seconds        = var.readiness_period_seconds
            success_threshold     = var.readiness_probe_success_threshold
            failure_threshold     = var.readiness_probe_failure_threshold
          }
        }
      }
    }
  }

  depends_on = [var.namespace_resource]
}
