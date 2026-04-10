# KAI-217: ElastiCache Redis for Kaivue cloud control plane.
#
# Backs the session/nonce store used by StreamClaims JWT validation
# (KAI-256 nonce bloom, KAI-257 single-use nonce). Must be:
#   - tenant-agnostic cache shared across the control plane (tenant_id
#     is embedded in the cache key by application code, not at the
#     infra layer)
#   - TLS in-transit + at-rest encrypted
#   - auth-token protected, with the token stored in Secrets Manager
#   - multi-AZ with automatic failover
#   - multi-region ready (one replication group per region; no
#     global-datastore coupling in v1)
#
# Parameter group is custom so we can pin maxmemory-policy, timeout,
# and enforce notify-keyspace-events for session expiry listeners.

locals {
  name_prefix = "kaivue-${var.environment}-${var.region}"

  module_tags = merge(var.tags, {
    Module      = "region/redis"
    Ticket      = "KAI-217"
    Environment = var.environment
    Region      = var.region
  })

  # redis7 family is required for TLS + auth-token + automatic failover
  # on cluster-mode-disabled replication groups used for session cache.
  engine_version = "7.1"
  family         = "redis7"
}

# ---------- Subnet + security groups ----------

resource "aws_elasticache_subnet_group" "main" {
  name       = local.name_prefix
  subnet_ids = var.subnet_ids
  tags       = local.module_tags
}

resource "aws_security_group" "redis" {
  name        = "${local.name_prefix}-redis"
  description = "Redis access from EKS nodes only"
  vpc_id      = var.vpc_id
  tags        = local.module_tags
}

resource "aws_security_group_rule" "redis_ingress_eks" {
  for_each = toset(var.allowed_security_group_ids)

  type                     = "ingress"
  from_port                = 6379
  to_port                  = 6379
  protocol                 = "tcp"
  security_group_id        = aws_security_group.redis.id
  source_security_group_id = each.value
  description              = "Redis 6379 from EKS node group ${each.value}"
}

resource "aws_security_group_rule" "redis_egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.redis.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow all egress (ElastiCache returns errors as RESP)"
}

# ---------- Parameter group ----------

resource "aws_elasticache_parameter_group" "main" {
  name        = "${local.name_prefix}-redis"
  family      = local.family
  description = "Kaivue session/nonce cache parameters (KAI-217)"

  parameter {
    name  = "maxmemory-policy"
    value = var.maxmemory_policy
  }

  # Close idle client sockets after 5m to keep connection count bounded.
  parameter {
    name  = "timeout"
    value = tostring(var.client_idle_timeout_seconds)
  }

  # Expired-key notifications are required by the nonce bloom rotation
  # worker (KAI-256) so it can prune its in-memory shadow set.
  parameter {
    name  = "notify-keyspace-events"
    value = "Ex"
  }

  tags = local.module_tags

  lifecycle {
    create_before_destroy = true
  }
}

# ---------- Auth token in Secrets Manager ----------
#
# ElastiCache auth-token requirements: 16-128 chars, printable ASCII,
# excludes @ " /. We generate locally and store in Secrets Manager so
# IRSA roles in the control plane can read it at runtime without ever
# landing in Terraform state in plaintext beyond what aws_secretsmanager_*
# already protects.

resource "random_password" "auth_token" {
  length           = 64
  special          = true
  override_special = "!#$%&()*+,-.:;<=>?[]^_{|}~"
}

resource "aws_secretsmanager_secret" "auth_token" {
  name                    = "${local.name_prefix}-redis-auth-token"
  description             = "ElastiCache Redis AUTH token for ${local.name_prefix}"
  kms_key_id              = var.kms_key_arn
  recovery_window_in_days = var.secret_recovery_window_days
  tags                    = local.module_tags
}

resource "aws_secretsmanager_secret_version" "auth_token" {
  secret_id     = aws_secretsmanager_secret.auth_token.id
  secret_string = random_password.auth_token.result
}

# ---------- Replication group ----------

resource "aws_elasticache_replication_group" "main" {
  replication_group_id = local.name_prefix
  description          = "Kaivue session/nonce cache (${var.environment} ${var.region})"

  engine         = "redis"
  engine_version = local.engine_version
  node_type      = var.redis_node_type
  port           = 6379

  num_cache_clusters         = var.num_cache_clusters
  automatic_failover_enabled = true
  multi_az_enabled           = var.num_cache_clusters > 1

  parameter_group_name = aws_elasticache_parameter_group.main.name
  subnet_group_name    = aws_elasticache_subnet_group.main.name
  security_group_ids   = [aws_security_group.redis.id]

  # Encryption: at-rest via KMS, in-transit TLS + auth token.
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  kms_key_id                 = var.kms_key_arn
  auth_token                 = random_password.auth_token.result
  auth_token_update_strategy = "ROTATE"
  # Rotation cadence: 90 days per SOC 2 CC6.1 automated credential
  # rotation policy (lead-security approved 2026-04-08). Rotation
  # lambda + Secrets Manager schedule ships as GA-blocker follow-up
  # paired with KAI-232 CI/CD.

  # Maintenance + backups.
  maintenance_window         = var.maintenance_window
  snapshot_window            = var.snapshot_window
  snapshot_retention_limit   = var.snapshot_retention_days
  apply_immediately          = var.environment != "prod"
  auto_minor_version_upgrade = true

  # CloudWatch Logs delivery for the slow log (redis engine log requires
  # engine 6.x+; 7.x supports both slow-log and engine-log destinations).
  log_delivery_configuration {
    destination      = aws_cloudwatch_log_group.redis_slow.name
    destination_type = "cloudwatch-logs"
    log_format       = "json"
    log_type         = "slow-log"
  }

  log_delivery_configuration {
    destination      = aws_cloudwatch_log_group.redis_engine.name
    destination_type = "cloudwatch-logs"
    log_format       = "json"
    log_type         = "engine-log"
  }

  tags = local.module_tags

  lifecycle {
    ignore_changes = [
      # auth_token rotation is managed out-of-band via
      # auth_token_update_strategy = ROTATE; let Terraform observe but
      # not thrash the value.
      auth_token,
    ]
  }
}

# ---------- CloudWatch log groups ----------

resource "aws_cloudwatch_log_group" "redis_slow" {
  name              = "/aws/elasticache/${local.name_prefix}/slow-log"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.kms_key_arn
  tags              = local.module_tags
}

resource "aws_cloudwatch_log_group" "redis_engine" {
  name              = "/aws/elasticache/${local.name_prefix}/engine-log"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.kms_key_arn
  tags              = local.module_tags
}

# ---------- CloudWatch alarms ----------
#
# Baseline alarms for the session cache. Thresholds are intentionally
# conservative because the nonce store is on the JWT hot path: a sick
# Redis manifests as 5xx on every authenticated request.

resource "aws_cloudwatch_metric_alarm" "redis_cpu" {
  alarm_name          = "${local.name_prefix}-redis-cpu-high"
  alarm_description   = "ElastiCache EngineCPUUtilization > ${var.cpu_alarm_threshold}%"
  namespace           = "AWS/ElastiCache"
  metric_name         = "EngineCPUUtilization"
  statistic           = "Average"
  period              = 60
  evaluation_periods  = 5
  threshold           = var.cpu_alarm_threshold
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ReplicationGroupId = aws_elasticache_replication_group.main.id
  }

  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "redis_memory" {
  alarm_name          = "${local.name_prefix}-redis-memory-high"
  alarm_description   = "ElastiCache DatabaseMemoryUsagePercentage > ${var.memory_alarm_threshold}%"
  namespace           = "AWS/ElastiCache"
  metric_name         = "DatabaseMemoryUsagePercentage"
  statistic           = "Average"
  period              = 60
  evaluation_periods  = 5
  threshold           = var.memory_alarm_threshold
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ReplicationGroupId = aws_elasticache_replication_group.main.id
  }

  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "redis_evictions" {
  alarm_name          = "${local.name_prefix}-redis-evictions"
  alarm_description   = "ElastiCache is evicting keys — capacity too small or maxmemory-policy too aggressive"
  namespace           = "AWS/ElastiCache"
  metric_name         = "Evictions"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 2
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ReplicationGroupId = aws_elasticache_replication_group.main.id
  }

  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "redis_replication_lag" {
  count = var.num_cache_clusters > 1 ? 1 : 0

  alarm_name          = "${local.name_prefix}-redis-replication-lag"
  alarm_description   = "ElastiCache ReplicationLag > ${var.replication_lag_alarm_seconds}s"
  namespace           = "AWS/ElastiCache"
  metric_name         = "ReplicationLag"
  statistic           = "Average"
  period              = 60
  evaluation_periods  = 3
  threshold           = var.replication_lag_alarm_seconds
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ReplicationGroupId = aws_elasticache_replication_group.main.id
  }

  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}
