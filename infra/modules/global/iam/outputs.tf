output "ci_role_arn" {
  description = "IAM role ARN assumed by GitHub Actions (KAI-232)"
  value       = aws_iam_role.ci.arn
}

output "readonly_role_arn" {
  description = "IAM role ARN for read-only console access"
  value       = aws_iam_role.readonly.arn
}

output "eks_admin_role_arn" {
  description = "IAM role ARN for EKS cluster administration (KAI-215)"
  value       = aws_iam_role.eks_admin.arn
}

output "terraform_state_role_arn" {
  description = "IAM role ARN for Terraform state access"
  value       = aws_iam_role.terraform_state.arn
}

output "break_glass_role_arn" {
  description = "IAM role ARN for emergency break-glass admin access"
  value       = aws_iam_role.break_glass.arn
}
