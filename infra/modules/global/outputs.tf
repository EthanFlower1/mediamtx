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

output "eks_admin_role_arn" {
  description = "IAM role ARN for EKS cluster administration (KAI-215)"
  value       = module.iam.eks_admin_role_arn
}

output "break_glass_role_arn" {
  description = "IAM role ARN for emergency break-glass admin access"
  value       = module.iam.break_glass_role_arn
}

output "organization_id" {
  description = "AWS Organizations organization ID"
  value       = module.organizations.organization_id
}

output "ou_workloads_production_id" {
  description = "Production OU ID for account placement"
  value       = module.organizations.ou_workloads_production_id
}

output "ou_workloads_staging_id" {
  description = "Staging OU ID for account placement"
  value       = module.organizations.ou_workloads_staging_id
}

output "cloudtrail_arn" {
  description = "Organization CloudTrail ARN"
  value       = module.baseline.cloudtrail_arn
}

output "guardduty_detector_id" {
  description = "GuardDuty detector ID"
  value       = module.baseline.guardduty_detector_id
}

output "terraform_state_bucket" {
  description = "S3 bucket for Terraform state"
  value       = module.baseline.terraform_state_bucket
}

output "terraform_locks_table" {
  description = "DynamoDB table for Terraform state locks"
  value       = module.baseline.terraform_locks_table
}
