output "api_key_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the Stripe API key (KAI-361)"
  value       = aws_secretsmanager_secret.stripe_api_key.arn
}

output "webhook_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the Stripe webhook secret"
  value       = aws_secretsmanager_secret.stripe_webhook_secret.arn
}
