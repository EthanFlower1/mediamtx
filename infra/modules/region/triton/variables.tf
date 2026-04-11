# KAI-277: Triton module variables.

variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "oidc_provider_arn" {
  description = "OIDC provider ARN for IRSA (from EKS module)"
  type        = string
}

variable "oidc_provider_url" {
  description = "OIDC provider URL without https:// (from EKS module)"
  type        = string
}

variable "kms_key_arn" {
  description = "KMS key ARN for S3 encryption"
  type        = string
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
