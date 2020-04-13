output "ratel_public_dns" {
  value = kubernetes_service.dgraph_ratel_public.load_balancer_ingress[0].hostname
}
