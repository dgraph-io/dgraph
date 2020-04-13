output "zero_public_dns" {
  value = kubernetes_service.dgraph_zero_public.load_balancer_ingress[0].hostname
}
