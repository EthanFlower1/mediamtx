output "hosted_zone_id" {
  description = "Route53 public hosted zone ID for the root domain"
  value       = module.route53.hosted_zone_id
}

output "api_fqdn" {
  description = "Map of region -> API FQDN (e.g. us-east-2.api.yourbrand.com)"
  value       = module.route53.api_fqdns
}

output "ci_role_arn" {
  description = "IAM role ARN assumed by CI/CD to deploy (KAI-232)"
  value       = module.iam.ci_role_arn
}
