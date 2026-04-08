# KAI-231: Production us-east-2 — instantiates the region module.
# To add eu-west-1 in v1.x: copy this directory to
# environments/production/regions/eu-west-1/, update terraform.tfvars,
# and run terraform apply. No module code changes required.

module "region" {
  source = "../../../../modules/region"

  region         = "us-east-2"
  environment    = "production"
  aws_account_id = var.aws_account_id

  cluster_version    = "1.29"
  db_instance_class  = "db.r7g.large"
  redis_node_type    = "cache.r7g.large"
  vpc_cidr           = "10.0.0.0/16"
  availability_zones = ["us-east-2a", "us-east-2b", "us-east-2c"]

  tags = {
    Project     = "kaivue"
    CostCenter  = "cloud-platform"
    Environment = "production"
    Region      = "us-east-2"
  }
}
