variable "region" {
  description = "AWS region for Secrets Manager resources"
  type        = string
}

variable "account_id" {
  description = "AWS account ID for resource ARN construction"
  type        = string
}

variable "tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default     = {}
}
