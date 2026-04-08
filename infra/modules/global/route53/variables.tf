variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "root_domain" {
  description = "Root DNS domain"
  type        = string
}

variable "active_regions" {
  description = "Active AWS regions — one A-record per region"
  type        = list(string)
}

variable "region_alb_dns" {
  description = "Map of region -> ALB DNS name"
  type        = map(string)
  default     = {}
}

variable "region_alb_zone_id" {
  description = "Map of region -> ALB hosted zone ID"
  type        = map(string)
  default     = {}
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
