variable "base_domain" {
  description = "Base domain for the platform (e.g. kaivue.io)"
  type        = string
  default     = "kaivue.io"
}

variable "origin_domain" {
  description = "Origin domain for the CloudFront distribution (e.g. app.kaivue.io)"
  type        = string
}

variable "tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default     = {}
}
