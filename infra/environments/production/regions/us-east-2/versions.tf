terraform {
  required_version = ">= 1.5.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }

  # TODO: configure remote state
  # backend "s3" {
  #   bucket         = "kaivue-production-tfstate"
  #   key            = "regions/us-east-2/terraform.tfstate"
  #   region         = "us-east-2"
  #   encrypt        = true
  #   dynamodb_table = "kaivue-production-tflock"
  # }
}

provider "aws" {
  region = "us-east-2"
  # aws_account_id enforced via assume_role in CI — never hardcode here.
  # allowed_account_ids = [var.aws_account_id]
}
