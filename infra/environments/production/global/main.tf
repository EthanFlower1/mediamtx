# KAI-231: Production global stack.
# Wire in per-region ALB outputs when deploying after the region stack.
# In v1.x, add new regions to active_regions and supply their ALB DNS/zone_id.

module "global" {
  source = "../../../modules/global"

  environment    = "production"
  aws_account_id = var.aws_account_id
  root_domain    = var.root_domain
  active_regions = ["us-east-2"]

  # Wire outputs from regions/us-east-2 stack (via remote state or manual input):
  region_alb_dns = {
    # "us-east-2" = "<alb-dns-from-region-stack>"
    # "eu-west-1" = "<alb-dns-from-eu-region-stack>"  # uncomment in v1.x
  }

  region_alb_zone_id = {
    # "us-east-2" = "<alb-zone-id-from-region-stack>"
  }

  tags = {
    Project    = "kaivue"
    CostCenter = "cloud-platform"
  }
}
