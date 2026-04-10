output "cloudtrail_arn" {
  description = "Organization CloudTrail ARN"
  value       = aws_cloudtrail.org_trail.arn
}

output "cloudtrail_s3_bucket" {
  description = "S3 bucket name for CloudTrail logs"
  value       = aws_s3_bucket.cloudtrail.id
}

output "guardduty_detector_id" {
  description = "GuardDuty detector ID"
  value       = aws_guardduty_detector.primary.id
}

output "terraform_state_bucket" {
  description = "S3 bucket name for Terraform state"
  value       = aws_s3_bucket.terraform_state.id
}

output "terraform_locks_table" {
  description = "DynamoDB table name for Terraform state locks"
  value       = aws_dynamodb_table.terraform_locks.name
}
