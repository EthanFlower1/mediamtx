output "kms_key_arn" {
  description = "ARN of the KMS key used to encrypt credential vault secrets"
  value       = aws_kms_key.credential_vault.arn
}

output "kms_key_alias" {
  description = "Alias of the KMS key"
  value       = aws_kms_alias.credential_vault.name
}

output "iam_policy_arn" {
  description = "ARN of the IAM policy granting credential vault access"
  value       = aws_iam_policy.credential_vault_access.arn
}
