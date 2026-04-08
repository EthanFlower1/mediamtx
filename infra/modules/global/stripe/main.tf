# KAI-231: Stripe Connect platform account stub.
# KAI-361 (Stripe Connect) will own the live implementation.
# This module stores Stripe API keys and webhook secrets in AWS Secrets Manager
# so the billing service (KAI-362) can retrieve them at runtime.
# Webhook endpoint registration is handled by the Stripe Terraform provider
# (not yet a required_provider here — added in KAI-361).

resource "aws_secretsmanager_secret" "stripe_api_key" {
  name                    = "kaivue/${var.environment}/stripe/api-key"
  description             = "Stripe Connect platform API key (KAI-361)"
  recovery_window_in_days = 7
  tags                    = var.tags
}

resource "aws_secretsmanager_secret" "stripe_webhook_secret" {
  name                    = "kaivue/${var.environment}/stripe/webhook-secret"
  description             = "Stripe webhook endpoint signing secret (KAI-361)"
  recovery_window_in_days = 7
  tags                    = var.tags
}
