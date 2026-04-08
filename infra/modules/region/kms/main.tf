# KAI-231: KMS sub-module stub.
# Real key policies will be written by KAI-215 (EKS) and KAI-216 (RDS)
# when those modules are promoted from stubs to live resources.

resource "aws_kms_key" "eks" {
  description             = "kaivue-${var.environment}-${var.region}-eks"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags                    = var.tags
}

resource "aws_kms_alias" "eks" {
  name          = "alias/kaivue-${var.environment}-${var.region}-eks"
  target_key_id = aws_kms_key.eks.key_id
}

resource "aws_kms_key" "rds" {
  description             = "kaivue-${var.environment}-${var.region}-rds"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags                    = var.tags
}

resource "aws_kms_alias" "rds" {
  name          = "alias/kaivue-${var.environment}-${var.region}-rds"
  target_key_id = aws_kms_key.rds.key_id
}

resource "aws_kms_key" "redis" {
  description             = "kaivue-${var.environment}-${var.region}-redis"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags                    = var.tags
}

resource "aws_kms_alias" "redis" {
  name          = "alias/kaivue-${var.environment}-${var.region}-redis"
  target_key_id = aws_kms_key.redis.key_id
}

resource "aws_kms_key" "secrets" {
  description             = "kaivue-${var.environment}-${var.region}-secrets"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags                    = var.tags
}

resource "aws_kms_alias" "secrets" {
  name          = "alias/kaivue-${var.environment}-${var.region}-secrets"
  target_key_id = aws_kms_key.secrets.key_id
}
