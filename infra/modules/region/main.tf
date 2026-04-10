# KAI-231: Region module root — composes all per-region sub-modules.
# Each sub-module is a stub that terraform validate passes against.
# Real provisioning is gated behind KAI-215 (EKS), KAI-216 (RDS),
# KAI-217 (Redis), KAI-220 (Zitadel), KAI-232 (CI/CD pipelines).
#
# ARCHITECTURAL RULE: Never put cross-region resources here.
# Cross-region resources (Route53, IAM roles, Stripe platform) live in
# modules/global/. See docs/proto-lock.md for the analogous lock protocol
# that governs schema changes across regions.

locals {
  common_tags = merge(var.tags, {
    Region      = var.region
    Environment = var.environment
    ManagedBy   = "terraform"
    KAITicket   = "KAI-231"
  })
}

module "kms" {
  source = "./kms"

  region         = var.region
  environment    = var.environment
  aws_account_id = var.aws_account_id
  tags           = local.common_tags
}

module "vpc" {
  source = "./vpc"

  region             = var.region
  environment        = var.environment
  vpc_cidr           = var.vpc_cidr
  availability_zones = var.availability_zones
  tags               = local.common_tags
}

module "eks" {
  source = "./eks"

  region          = var.region
  environment     = var.environment
  cluster_version = var.cluster_version
  vpc_id          = module.vpc.vpc_id
  subnet_ids      = module.vpc.private_subnet_ids
  kms_key_arn     = module.kms.eks_key_arn
  tags            = local.common_tags

  depends_on = [module.vpc, module.kms]
}

module "rds" {
  source = "./rds"

  region            = var.region
  environment       = var.environment
  db_instance_class = var.db_instance_class
  vpc_id            = module.vpc.vpc_id
  subnet_ids        = module.vpc.private_subnet_ids
  kms_key_arn       = module.kms.rds_key_arn
  tags              = local.common_tags

  depends_on = [module.vpc, module.kms]
}

module "redis" {
  source = "./redis"

  region          = var.region
  environment     = var.environment
  redis_node_type = var.redis_node_type
  vpc_id          = module.vpc.vpc_id
  subnet_ids      = module.vpc.private_subnet_ids
  kms_key_arn     = module.kms.redis_key_arn
  tags            = local.common_tags

  depends_on = [module.vpc, module.kms]
}

module "alb" {
  source = "./alb"

  region      = var.region
  environment = var.environment
  vpc_id      = module.vpc.vpc_id
  subnet_ids  = module.vpc.public_subnet_ids
  tags        = local.common_tags

  depends_on = [module.vpc]
}

module "cloudfront" {
  source = "./cloudfront"

  region      = var.region
  environment = var.environment
  alb_dns     = module.alb.dns_name
  tags        = local.common_tags

  depends_on = [module.alb]
}

module "zitadel" {
  source = "./zitadel"

  region              = var.region
  environment         = var.environment
  external_domain     = var.zitadel_external_domain
  db_host             = module.rds.cluster_endpoint
  db_admin_secret_arn = module.rds.admin_secret_arn
  masterkey_secret_arn = var.zitadel_masterkey_secret_arn
  tags                = local.common_tags

  depends_on = [module.eks, module.rds]
}
