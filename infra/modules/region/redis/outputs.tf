output "primary_endpoint" {
  description = "Redis primary endpoint address (TLS, port 6379)"
  value       = aws_elasticache_replication_group.main.primary_endpoint_address
  sensitive   = true
}

output "reader_endpoint" {
  description = "Redis reader endpoint address (TLS, port 6379)"
  value       = aws_elasticache_replication_group.main.reader_endpoint_address
  sensitive   = true
}

output "port" {
  description = "Redis port"
  value       = aws_elasticache_replication_group.main.port
}

output "replication_group_id" {
  description = "ElastiCache replication group ID"
  value       = aws_elasticache_replication_group.main.id
}

output "replication_group_arn" {
  description = "ElastiCache replication group ARN (for IAM policies / CloudWatch targeting)"
  value       = aws_elasticache_replication_group.main.arn
}

output "security_group_id" {
  description = "Security group ID for Redis ingress rule management"
  value       = aws_security_group.redis.id
}

output "parameter_group_name" {
  description = "ElastiCache parameter group name"
  value       = aws_elasticache_parameter_group.main.name
}

output "auth_token_secret_arn" {
  description = "Secrets Manager ARN holding the Redis AUTH token (read via IRSA at runtime)"
  value       = aws_secretsmanager_secret.auth_token.arn
}

output "auth_token_secret_name" {
  description = "Secrets Manager secret name for the Redis AUTH token"
  value       = aws_secretsmanager_secret.auth_token.name
}

output "slow_log_group_name" {
  description = "CloudWatch log group receiving the Redis slow log"
  value       = aws_cloudwatch_log_group.redis_slow.name
}

output "engine_log_group_name" {
  description = "CloudWatch log group receiving the Redis engine log"
  value       = aws_cloudwatch_log_group.redis_engine.name
}
