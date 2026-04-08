# KAI-231: Global module root — composes cross-region sub-modules.
# Anything here must be region-agnostic. Region-scoped stacks
# instantiate modules/region/ once per region directory under
# environments/<env>/regions/<region>/.

locals {
  common_tags = merge(var.tags, {
    Environment = var.environment
    ManagedBy   = "terraform"
    KAITicket   = "KAI-231"
  })
}

module "route53" {
  source = "./route53"

  environment        = var.environment
  root_domain        = var.root_domain
  active_regions     = var.active_regions
  region_alb_dns     = var.region_alb_dns
  region_alb_zone_id = var.region_alb_zone_id
  tags               = local.common_tags
}

module "iam" {
  source = "./iam"

  environment    = var.environment
  aws_account_id = var.aws_account_id
  tags           = local.common_tags
}

module "organizations" {
  source = "./organizations"

  environment = var.environment
  tags        = local.common_tags
}

module "stripe" {
  source = "./stripe"

  environment = var.environment
  tags        = local.common_tags
}
