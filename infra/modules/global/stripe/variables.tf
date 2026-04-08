# KAI-361: Stripe Connect platform account config (stub).
# Billing modes: direct (platform -> customer) and via_integrator
# (platform -> integrator at wholesale -> customer at markup).

variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
