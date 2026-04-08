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
  #   bucket         = "kaivue-staging-tfstate"
  #   key            = "global/terraform.tfstate"
  #   region         = "us-east-2"
  #   encrypt        = true
  #   dynamodb_table = "kaivue-staging-tflock"
  # }
}

provider "aws" {
  region = "us-east-2"
}
