# KAI-277: Triton module outputs.

output "triton_role_arn" {
  description = "IAM role ARN for Triton service account (IRSA)"
  value       = aws_iam_role.triton.arn
}

output "model_repository_bucket" {
  description = "S3 bucket name for the Triton model repository"
  value       = aws_s3_bucket.model_repository.id
}

output "model_repository_arn" {
  description = "S3 bucket ARN for the Triton model repository"
  value       = aws_s3_bucket.model_repository.arn
}
