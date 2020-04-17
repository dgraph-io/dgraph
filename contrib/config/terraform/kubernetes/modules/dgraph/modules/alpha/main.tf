locals {
  alpha_replicas        = var.ha ? var.replicas : 1
  has_pod_anti_affinity = var.ha ? [1] : []
  has_persistence       = var.persistence ? [1] : []
  initialize_data       = var.initialize_data ? [1] : []
}


resource "kubernetes_service" "dgraph_alpha_public" {
  metadata {
    name      = "${var.prefix}-dgraph-alpha-public"
    namespace = var.namespace

    labels = {
      app     = "${var.prefix}-dgraph-alpha"
      monitor = "alpha-dgraph-io"
    }
  }

  spec {
    port {
      name        = "alpha-grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
    }
    port {
      name        = "alpha-http"
      port        = var.http_port
      target_port = var.http_port
    }

    selector = {
      app = "${var.prefix}-dgraph-alpha"
    }

    type = "LoadBalancer"

    load_balancer_source_ranges = var.ingress_whitelist_cidrs
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_service" "dgraph_alpha_indexzero_public" {
  metadata {
    name      = "${var.prefix}-dgraph-alpha-0-public"
    namespace = var.namespace

    labels = {
      app = "${var.prefix}-dgraph-alpha"
    }
  }

  spec {
    port {
      name        = "alpha-http"
      port        = var.http_port
      target_port = var.http_port
    }

    selector = {
      "statefulset.kubernetes.io/pod-name" = "${var.prefix}-dgraph-alpha-0"
    }

    type = "LoadBalancer"

    load_balancer_source_ranges = var.ingress_whitelist_cidrs
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_service" "dgraph_alpha" {
  metadata {
    name      = "${var.prefix}-dgraph-alpha"
    namespace = var.namespace

    labels = {
      app = "${var.prefix}-dgraph-alpha"
    }
  }

  spec {
    port {
      name        = "alpha-grpc-int"
      port        = var.grpc_int_port
      target_port = var.grpc_int_port
    }

    selector = {
      app = "${var.prefix}-dgraph-alpha"
    }

    type                        = "ClusterIP"
    cluster_ip                  = "None"
    publish_not_ready_addresses = true
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_config_map" "dgraph_alpha_command" {
  metadata {
    name      = "${var.prefix}-dgraph-alpha-cm"
    namespace = var.namespace
  }

  data = {
    "dgraph_alpha.sh"      = templatefile("${path.module}/templates/alpha.tpl", { prefix = var.prefix, namespace = var.namespace, replicas = local.alpha_replicas, lru = var.lru_size_mb, zero_address = var.zero_address })
    "dgraph_init_alpha.sh" = file("${path.module}/templates/alpha_init.sh")
  }

  depends_on = [var.namespace_resource]
}


resource "kubernetes_stateful_set" "dgraph_alpha" {
  metadata {
    name      = "${var.prefix}-dgraph-alpha"
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


    service_name = "${var.prefix}-dgraph-alpha"

    replicas = local.alpha_replicas

    selector {
      match_labels = {
        app = "${var.prefix}-dgraph-alpha"
      }
    }

    template {
      metadata {
        labels = {
          app = "${var.prefix}-dgraph-alpha"
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
                      values   = ["${var.prefix}-dgraph-alpha"]
                    }
                  }
                  topology_key = "kubernetes.io/hostname"
                }
              }
            }
          }
        }

        termination_grace_period_seconds = var.termination_grace_period_seconds

        volume {
          name = "dgraph-alpha-command"

          config_map {
            name         = "${var.prefix}-dgraph-alpha-cm"
            default_mode = "0755"
          }
        }

        dynamic "init_container" {
          for_each = local.initialize_data
          content {
            name = "init-alpha"

            image             = "${var.image}:${var.image_version}"
            image_pull_policy = var.image_pull_policy

            command = var.init_command

            volume_mount {
              name       = "dgraph-alpha-command"
              mount_path = "/cmd"
            }

            dynamic "volume_mount" {
              for_each = local.has_persistence
              content {
                name       = "datadir"
                mount_path = var.persistence_mount_path
              }
            }
          }
        }

        container {
          name = "alpha"

          image             = "${var.image}:${var.image_version}"
          image_pull_policy = var.image_pull_policy

          env {
            name  = "POD_NAMESPACE"
            value = var.namespace
          }

          command = var.command

          port {
            name           = "alpha-grpc-int"
            container_port = var.grpc_int_port
          }

          port {
            name           = "alpha-http"
            container_port = var.http_port
          }

          port {
            name           = "alpha-grpc"
            container_port = var.grpc_port
          }

          dynamic "volume_mount" {
            for_each = local.has_persistence
            content {
              name       = "datadir"
              mount_path = var.persistence_mount_path
            }
          }

          volume_mount {
            name       = "dgraph-alpha-command"
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
