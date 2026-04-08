# KAI-231: Dev us-east-2 — minimal instance sizes to keep costs low.

module "region" {
  source = "../../../../modules/region"

  region         = "us-east-2"
  environment    = "dev"
  aws_account_id = var.aws_account_id

  cluster_version    = "1.29"
  db_instance_class  = "db.t3.micro"
  redis_node_type    = "cache.t3.micro"
  vpc_cidr           = "10.2.0.0/16"
  availability_zones = ["us-east-2a", "us-east-2b"]

  tags = {
    Project     = "kaivue"
    CostCenter  = "cloud-platform"
    Environment = "dev"
    Region      = "us-east-2"
  }
}
