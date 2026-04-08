# KAI-231: Per-region Terraform module structure.
# All region-scoped resources are parameterized by these variables so that
# adding region #2 in v1.x is "uncomment a directory + terraform apply".

variable "region" {
  description = "AWS region for this deployment (e.g. us-east-2, eu-west-1)"
  type        = string
}

variable "environment" {
  description = "Deployment environment: dev | staging | production"
  type        = string

  validation {
    condition     = contains(["dev", "staging", "production"], var.environment)
    error_message = "environment must be dev, staging, or production"
  }
}

variable "aws_account_id" {
  description = "AWS account ID (never hardcode; always pass via tfvars or CI secrets)"
  type        = string
}

variable "cluster_version" {
  description = "Kubernetes version to run on EKS (KAI-215)"
  type        = string
  default     = "1.29"
}

variable "db_instance_class" {
  description = "RDS instance class (KAI-216)"
  type        = string
  default     = "db.t3.medium"
}

variable "redis_node_type" {
  description = "ElastiCache node type (KAI-217)"
  type        = string
  default     = "cache.t3.micro"
}

variable "vpc_cidr" {
  description = "CIDR block for the region VPC (KAI-215)"
  type        = string
  default     = "10.0.0.0/16"
}

variable "availability_zones" {
  description = "List of AZs to use within the region (minimum 2 for HA)"
  type        = list(string)
}

variable "tags" {
  description = "Common tags applied to all resources in this region module"
  type        = map(string)
  default     = {}
}
