output "eks_key_arn" {
  description = "KMS key ARN for EKS secrets encryption"
  value       = aws_kms_key.eks.arn
}

output "rds_key_arn" {
  description = "KMS key ARN for RDS storage encryption"
  value       = aws_kms_key.rds.arn
}

output "redis_key_arn" {
  description = "KMS key ARN for ElastiCache at-rest encryption"
  value       = aws_kms_key.redis.arn
}

output "secrets_key_arn" {
  description = "KMS key ARN for Secrets Manager entries"
  value       = aws_kms_key.secrets.arn
}
