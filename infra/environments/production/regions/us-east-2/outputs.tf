output "eks_cluster_name" {
  value = module.region.eks_cluster_name
}

output "rds_cluster_endpoint" {
  value     = module.region.rds_cluster_endpoint
  sensitive = true
}

output "alb_dns_name" {
  value = module.region.alb_dns_name
}

output "cloudfront_domain" {
  value = module.region.cloudfront_domain
}
