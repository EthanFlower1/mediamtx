variable "region" {
  description = "AWS region (CloudFront itself is global but origin is regional)"
  type        = string
}

variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "alb_dns" {
  description = "ALB DNS name used as the CloudFront origin"
  type        = string
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
