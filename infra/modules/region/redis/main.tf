# KAI-231: ElastiCache Redis sub-module stub.
# KAI-217 will fill in: cluster mode, auth token from Secrets Manager,
# TLS in-transit enforcement, automatic failover, and the session/nonce
# store used by StreamClaims JWT validation (KAI-256/257).

resource "aws_elasticache_subnet_group" "main" {
  name       = "kaivue-${var.environment}-${var.region}"
  subnet_ids = var.subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "redis" {
  name        = "kaivue-${var.environment}-${var.region}-redis"
  description = "Redis access from EKS nodes only"
  vpc_id      = var.vpc_id
  tags        = var.tags
}

resource "aws_elasticache_replication_group" "main" {
  replication_group_id       = "kaivue-${var.environment}-${var.region}"
  description                = "Kaivue session/nonce cache (${var.environment} ${var.region})"
  node_type                  = var.redis_node_type
  num_cache_clusters         = 2
  automatic_failover_enabled = true
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  kms_key_id                 = var.kms_key_arn
  subnet_group_name          = aws_elasticache_subnet_group.main.name
  security_group_ids         = [aws_security_group.redis.id]

  tags = var.tags
}
