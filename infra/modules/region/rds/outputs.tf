output "cluster_endpoint" {
  description = "RDS primary endpoint"
  value       = aws_db_instance.primary.endpoint
  sensitive   = true
}

output "reader_endpoint" {
  description = "RDS reader endpoint (same as primary for single-AZ; read replica added in KAI-216)"
  value       = aws_db_instance.primary.endpoint
  sensitive   = true
}

output "db_instance_id" {
  description = "RDS instance ID"
  value       = aws_db_instance.primary.identifier
}

output "db_security_group_id" {
  description = "Security group ID attached to RDS (for EKS IRSA ingress rules)"
  value       = aws_security_group.rds.id
}

output "admin_secret_arn" {
  description = "ARN of the AWS-managed Secrets Manager secret for the RDS admin password (KAI-220)"
  value       = aws_db_instance.primary.master_user_secret[0].secret_arn
  sensitive   = true
}
