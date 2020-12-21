output "alpha_public_dns" {
  value = kubernetes_service.dgraph_alpha_public.load_balancer_ingress[0].hostname
}

output "alpha_indexzero_public_dns" {
  value = kubernetes_service.dgraph_alpha_indexzero_public.load_balancer_ingress[0].hostname
}
