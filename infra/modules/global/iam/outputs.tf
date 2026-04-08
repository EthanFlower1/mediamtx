output "ci_role_arn" {
  description = "IAM role ARN assumed by GitHub Actions (KAI-232)"
  value       = aws_iam_role.ci.arn
}

output "readonly_role_arn" {
  description = "IAM role ARN for read-only console access"
  value       = aws_iam_role.readonly.arn
}
