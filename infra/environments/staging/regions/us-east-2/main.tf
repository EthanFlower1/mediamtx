# KAI-231: Staging us-east-2 — same module, smaller instance sizes.
# Environment logic never lives inside the module; it's expressed as tfvars here.

module "region" {
  source = "../../../../modules/region"

  region         = "us-east-2"
  environment    = "staging"
  aws_account_id = var.aws_account_id

  cluster_version    = "1.29"
  db_instance_class  = "db.t3.medium"
  redis_node_type    = "cache.t3.micro"
  vpc_cidr           = "10.1.0.0/16"
  availability_zones = ["us-east-2a", "us-east-2b"]

  tags = {
    Project     = "kaivue"
    CostCenter  = "cloud-platform"
    Environment = "staging"
    Region      = "us-east-2"
  }
}
