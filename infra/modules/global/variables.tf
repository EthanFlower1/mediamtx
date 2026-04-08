# KAI-231: Global module variables.
# Cross-region resources live here; region-scoped resources never do.

variable "environment" {
  description = "Deployment environment: dev | staging | production"
  type        = string

  validation {
    condition     = contains(["dev", "staging", "production"], var.environment)
    error_message = "environment must be dev, staging, or production"
  }
}

variable "aws_account_id" {
  description = "AWS account ID"
  type        = string
}

variable "root_domain" {
  description = "Root DNS domain (e.g. yourbrand.com)"
  type        = string
}

variable "active_regions" {
  description = "List of active AWS regions; used to build per-region DNS records"
  type        = list(string)
  default     = ["us-east-2"]
}

variable "region_alb_dns" {
  description = "Map of region -> ALB DNS name (passed in from per-region outputs)"
  type        = map(string)
  default     = {}
}

variable "region_alb_zone_id" {
  description = "Map of region -> ALB hosted zone ID (for Route53 alias records)"
  type        = map(string)
  default     = {}
}

variable "tags" {
  description = "Common tags applied to all global resources"
  type        = map(string)
  default     = {}
}
