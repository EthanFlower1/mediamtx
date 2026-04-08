# KAI-231: RDS sub-module stub.
# KAI-216 will fill in: pgvector extension, parameter group tuning,
# performance insights, automated backups, enhanced monitoring,
# cross-region read replica for active-active v1.x expansion,
# and the multi-tenant schema migration baseline (KAI-218).
# Every tenant query MUST be scoped by tenant_id and region columns (KAI-218/219).

resource "aws_db_subnet_group" "main" {
  name       = "kaivue-${var.environment}-${var.region}"
  subnet_ids = var.subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "rds" {
  name        = "kaivue-${var.environment}-${var.region}-rds"
  description = "RDS access from EKS nodes only"
  vpc_id      = var.vpc_id
  tags        = var.tags
}

resource "aws_db_instance" "primary" {
  identifier            = "kaivue-${var.environment}-${var.region}-primary"
  engine                = "postgres"
  engine_version        = "15.6"
  instance_class        = var.db_instance_class
  allocated_storage     = 20
  max_allocated_storage = 1000
  db_name               = "kaivue"
  username              = "kaivue_admin"
  # Password is injected at runtime from Secrets Manager; never hardcode.
  manage_master_user_password = true
  db_subnet_group_name        = aws_db_subnet_group.main.name
  vpc_security_group_ids      = [aws_security_group.rds.id]
  storage_encrypted           = true
  kms_key_id                  = var.kms_key_arn
  backup_retention_period     = 7
  deletion_protection         = true
  skip_final_snapshot         = false
  final_snapshot_identifier   = "kaivue-${var.environment}-${var.region}-final"

  tags = var.tags
}
