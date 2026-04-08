# KAI-231: Region module outputs consumed by the global module and by
# other services (e.g. CI, KAI-232) that need to locate regional resources.

output "eks_cluster_name" {
  description = "EKS cluster name (KAI-215)"
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS API server endpoint"
  value       = module.eks.cluster_endpoint
  sensitive   = true
}

output "rds_cluster_endpoint" {
  description = "RDS writer endpoint (KAI-216)"
  value       = module.rds.cluster_endpoint
  sensitive   = true
}

output "rds_reader_endpoint" {
  description = "RDS read-replica endpoint (cross-region reads in v1.x)"
  value       = module.rds.reader_endpoint
  sensitive   = true
}

output "redis_primary_endpoint" {
  description = "ElastiCache Redis primary endpoint (KAI-217)"
  value       = module.redis.primary_endpoint
  sensitive   = true
}

output "vpc_id" {
  description = "VPC ID for this region"
  value       = module.vpc.vpc_id
}

output "alb_dns_name" {
  description = "ALB DNS name (KAI-230 region routing)"
  value       = module.alb.dns_name
}

output "cloudfront_domain" {
  description = "CloudFront distribution domain"
  value       = module.cloudfront.domain_name
}

output "kms_eks_key_arn" {
  description = "KMS key ARN used for EKS secrets encryption"
  value       = module.kms.eks_key_arn
  sensitive   = true
}
