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

output "redis_reader_endpoint" {
  description = "ElastiCache Redis reader endpoint (KAI-217)"
  value       = module.redis.reader_endpoint
  sensitive   = true
}

output "redis_auth_token_secret_arn" {
  description = "Secrets Manager ARN for the Redis AUTH token (KAI-217; consumed by IRSA at runtime)"
  value       = module.redis.auth_token_secret_arn
  sensitive   = true
}

output "redis_security_group_id" {
  description = "Redis security group ID (for EKS node-group ingress rules from other modules)"
  value       = module.redis.security_group_id
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

# --- Zitadel (KAI-220) ---

output "zitadel_external_domain" {
  description = "Public domain for the Zitadel identity server"
  value       = module.zitadel.external_domain
}

output "zitadel_internal_endpoint" {
  description = "Cluster-internal gRPC endpoint for Zitadel"
  value       = module.zitadel.internal_endpoint
  sensitive   = true
}
