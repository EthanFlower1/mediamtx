# KAI-216: RDS Postgres 15 + pgvector + read replica + Performance Insights.
#
# This module deepens the KAI-231 stub:
#   * pgvector extension enabled via a custom parameter group and bootstrap
#     script (the extension itself is CREATE EXTENSION'd by the KAI-218
#     migration baseline — this module only makes it *available*).
#   * DB parameter group with multi-tenant-friendly tuning (work_mem,
#     shared_preload_libraries for pg_stat_statements + vector, autovacuum
#     thresholds, statement_timeout fail-safe).
#   * Performance Insights (7-day retention; longer retention is a cost
#     lever KAI-232 CI/CD will parameterise per environment).
#   * Enhanced monitoring IAM role + 60s granularity.
#   * Secrets Manager-managed master password with automatic rotation hook
#     (rotation lambda is wired in KAI-232).
#   * Cross-region read replica stub guarded by var.enable_read_replica so
#     v1 can ship single-region but the active-active v1.x expansion flips
#     a single flag.
#   * CloudWatch alarm baseline on storage + CPU + free memory + replica
#     lag (lag alarm suppressed when read replica disabled).
#
# Multi-tenant isolation (Seam #4) is enforced at the application layer
# (internal/cloud/ tenant filters); this module only needs to ensure the
# database is healthy, encrypted, and discoverable.

locals {
  # Keep the major version explicit. Upgrades are a deliberate operator
  # action, not something that should happen because a minor version
  # family bumped upstream.
  postgres_major = "15"
  postgres_full  = "15.6"
  family         = "postgres${local.postgres_major}"

  # Tags that flow to every child resource so cost allocation and
  # incident response can find everything by one filter.
  module_tags = merge(var.tags, {
    "kaivue:module"    = "region/rds"
    "kaivue:ticket"    = "KAI-216"
    "kaivue:component" = "cloud-control-plane"
  })
}

###############################################################################
# Networking plumbing
###############################################################################

resource "aws_db_subnet_group" "main" {
  name       = "kaivue-${var.environment}-${var.region}"
  subnet_ids = var.subnet_ids
  tags       = local.module_tags
}

resource "aws_security_group" "rds" {
  name        = "kaivue-${var.environment}-${var.region}-rds"
  description = "RDS Postgres access from EKS nodes only (no public ingress)."
  vpc_id      = var.vpc_id
  tags        = local.module_tags
}

# Ingress from the EKS worker security group on 5432 only. The EKS module
# (KAI-215) wires its node SG ID through var.allowed_security_group_ids so
# this module has no opinion about which cluster is allowed in.
resource "aws_security_group_rule" "rds_ingress_eks" {
  for_each                 = toset(var.allowed_security_group_ids)
  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  security_group_id        = aws_security_group.rds.id
  source_security_group_id = each.value
  description              = "Postgres ingress from EKS node SG ${each.value}"
}

###############################################################################
# Parameter group — tuning + pgvector + pg_stat_statements
###############################################################################

resource "aws_db_parameter_group" "main" {
  name        = "kaivue-${var.environment}-${var.region}-pg${local.postgres_major}"
  family      = local.family
  description = "Kaivue Postgres ${local.postgres_major} parameters (KAI-216)."

  # Load pgvector + pg_stat_statements at startup so they're available to
  # every session. pg_stat_statements powers KAI-422 slow-query dashboards;
  # vector powers KAI-292 per-tenant pgvector indexes.
  parameter {
    name         = "shared_preload_libraries"
    value        = "pg_stat_statements,vector"
    apply_method = "pending-reboot"
  }

  # Default work_mem (4MB) is too low for vector similarity scans. 32MB is
  # enough headroom for KAI-292 tenant-scoped HNSW queries without blowing
  # up total memory across connection pools.
  parameter {
    name  = "work_mem"
    value = "32768"
  }

  # Statement timeout is the safety rail for a runaway tenant query. The
  # application layer sets tighter per-endpoint timeouts; this is the
  # last-resort fail-safe.
  parameter {
    name  = "statement_timeout"
    value = tostring(var.statement_timeout_ms)
  }

  # Autovacuum tuned for high-churn multi-tenant tables (audit_log,
  # usage_events). Scale factors intentionally low so the vacuum chases
  # write volume instead of table size.
  parameter {
    name  = "autovacuum_vacuum_scale_factor"
    value = "0.02"
  }
  parameter {
    name  = "autovacuum_analyze_scale_factor"
    value = "0.01"
  }

  # Log anything slower than 1s so pg_stat_statements + CloudWatch Logs
  # capture it for the lead-sre dashboards (KAI-422).
  parameter {
    name  = "log_min_duration_statement"
    value = "1000"
  }

  # Connection count budget. Defaults scale with instance class; we pin a
  # ceiling so no one accidentally drains the pool with a runaway sidecar.
  parameter {
    name  = "max_connections"
    value = tostring(var.max_connections)
  }

  tags = local.module_tags

  lifecycle {
    create_before_destroy = true
  }
}

###############################################################################
# Enhanced Monitoring IAM role
###############################################################################

data "aws_iam_policy_document" "enhanced_monitoring_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["monitoring.rds.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "enhanced_monitoring" {
  name               = "kaivue-${var.environment}-${var.region}-rds-mon"
  assume_role_policy = data.aws_iam_policy_document.enhanced_monitoring_assume.json
  tags               = local.module_tags
}

resource "aws_iam_role_policy_attachment" "enhanced_monitoring" {
  role       = aws_iam_role.enhanced_monitoring.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}

###############################################################################
# Primary instance
###############################################################################

resource "aws_db_instance" "primary" {
  identifier     = "kaivue-${var.environment}-${var.region}-primary"
  engine         = "postgres"
  engine_version = local.postgres_full
  instance_class = var.db_instance_class

  allocated_storage     = var.allocated_storage_gb
  max_allocated_storage = var.max_allocated_storage_gb
  storage_type          = "gp3"
  storage_encrypted     = true
  kms_key_id            = var.kms_key_arn

  db_name  = "kaivue"
  username = "kaivue_admin"
  # Master password lives in Secrets Manager; rotation is wired via the
  # KAI-232 rotation lambda. Never hardcode, never template into TF.
  manage_master_user_password   = true
  master_user_secret_kms_key_id = var.kms_key_arn

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  parameter_group_name   = aws_db_parameter_group.main.name

  multi_az = var.multi_az

  backup_retention_period   = var.backup_retention_days
  backup_window             = "03:00-04:00"
  maintenance_window        = "sun:04:00-sun:05:00"
  copy_tags_to_snapshot     = true
  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "kaivue-${var.environment}-${var.region}-final"

  # Performance Insights for the query-level view lead-sre needs when
  # chasing a regression. 7 days is the free tier; KAI-232 will bump this
  # to 731 (2y) in prod.
  performance_insights_enabled          = true
  performance_insights_retention_period = var.performance_insights_retention_days
  performance_insights_kms_key_id       = var.kms_key_arn

  # Enhanced monitoring feeds 60s-granularity OS metrics to CloudWatch.
  monitoring_interval = 60
  monitoring_role_arn = aws_iam_role.enhanced_monitoring.arn

  # Stream Postgres + upgrade logs to CloudWatch so KAI-422 can scrape.
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]

  # Automatic minor version upgrades in non-prod only. Prod upgrades are
  # explicit change tickets.
  auto_minor_version_upgrade = var.environment != "prod"

  tags = local.module_tags

  lifecycle {
    # Secrets Manager manages the password — ignore any rotation churn in
    # the plan. Also ignore allocated_storage so storage autoscale events
    # do not surface as drift.
    ignore_changes = [
      password,
      allocated_storage,
    ]
  }
}

###############################################################################
# Read replica (optional, guarded by var.enable_read_replica)
###############################################################################

resource "aws_db_instance" "replica" {
  count = var.enable_read_replica ? 1 : 0

  identifier             = "kaivue-${var.environment}-${var.region}-replica"
  instance_class         = var.replica_instance_class
  replicate_source_db    = aws_db_instance.primary.identifier
  vpc_security_group_ids = [aws_security_group.rds.id]
  parameter_group_name   = aws_db_parameter_group.main.name

  performance_insights_enabled          = true
  performance_insights_retention_period = var.performance_insights_retention_days
  performance_insights_kms_key_id       = var.kms_key_arn

  monitoring_interval = 60
  monitoring_role_arn = aws_iam_role.enhanced_monitoring.arn

  auto_minor_version_upgrade = var.environment != "prod"
  skip_final_snapshot        = true
  deletion_protection        = true

  tags = local.module_tags
}

###############################################################################
# CloudWatch alarm baseline
###############################################################################

resource "aws_cloudwatch_metric_alarm" "primary_cpu" {
  alarm_name          = "kaivue-${var.environment}-${var.region}-rds-primary-cpu"
  alarm_description   = "RDS primary CPU > 85% for 10 minutes."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 85
  treat_missing_data  = "notBreaching"
  dimensions = {
    DBInstanceIdentifier = aws_db_instance.primary.identifier
  }
  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "primary_free_storage" {
  alarm_name          = "kaivue-${var.environment}-${var.region}-rds-primary-storage"
  alarm_description   = "RDS primary free storage below 10% of allocated."
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 2
  metric_name         = "FreeStorageSpace"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  # Bytes. 10% of allocated_storage_gb.
  threshold          = var.allocated_storage_gb * 1024 * 1024 * 1024 * 0.10
  treat_missing_data = "breaching"
  dimensions = {
    DBInstanceIdentifier = aws_db_instance.primary.identifier
  }
  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "primary_free_memory" {
  alarm_name          = "kaivue-${var.environment}-${var.region}-rds-primary-memory"
  alarm_description   = "RDS primary freeable memory < 200MB for 10 minutes."
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 2
  metric_name         = "FreeableMemory"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 200 * 1024 * 1024
  treat_missing_data  = "breaching"
  dimensions = {
    DBInstanceIdentifier = aws_db_instance.primary.identifier
  }
  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}

resource "aws_cloudwatch_metric_alarm" "replica_lag" {
  count               = var.enable_read_replica ? 1 : 0
  alarm_name          = "kaivue-${var.environment}-${var.region}-rds-replica-lag"
  alarm_description   = "RDS read replica lag > 30s."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "ReplicaLag"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 30
  treat_missing_data  = "notBreaching"
  dimensions = {
    DBInstanceIdentifier = aws_db_instance.replica[0].identifier
  }
  alarm_actions = var.alarm_sns_topic_arns
  ok_actions    = var.alarm_sns_topic_arns
  tags          = local.module_tags
}
