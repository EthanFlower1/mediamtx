# KAI-231: Dev global stack.

module "global" {
  source = "../../../modules/global"

  environment    = "dev"
  aws_account_id = var.aws_account_id
  root_domain    = var.root_domain
  active_regions = ["us-east-2"]

  region_alb_dns     = {}
  region_alb_zone_id = {}

  tags = {
    Project    = "kaivue"
    CostCenter = "cloud-platform"
  }
}
