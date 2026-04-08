output "primary_endpoint" {
  description = "Redis primary endpoint address"
  value       = aws_elasticache_replication_group.main.primary_endpoint_address
  sensitive   = true
}

output "reader_endpoint" {
  description = "Redis reader endpoint address"
  value       = aws_elasticache_replication_group.main.reader_endpoint_address
  sensitive   = true
}

output "security_group_id" {
  description = "Security group ID for Redis ingress rule management"
  value       = aws_security_group.redis.id
}
