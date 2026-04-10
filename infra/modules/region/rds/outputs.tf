# KAI-216: RDS module outputs.

output "primary_endpoint" {
  description = "RDS primary writer endpoint."
  value       = aws_db_instance.primary.endpoint
  sensitive   = true
}

output "primary_address" {
  description = "RDS primary hostname (no port)."
  value       = aws_db_instance.primary.address
  sensitive   = true
}

output "primary_port" {
  description = "RDS primary port."
  value       = aws_db_instance.primary.port
}

output "reader_endpoint" {
  description = "Read replica endpoint when enable_read_replica is true; primary endpoint otherwise."
  value       = var.enable_read_replica ? aws_db_instance.replica[0].endpoint : aws_db_instance.primary.endpoint
  sensitive   = true
}

output "db_instance_id" {
  description = "RDS primary instance identifier."
  value       = aws_db_instance.primary.identifier
}

output "db_instance_arn" {
  description = "RDS primary instance ARN (used by the rotation lambda IAM policy in KAI-232)."
  value       = aws_db_instance.primary.arn
}

output "db_security_group_id" {
  description = "Security group ID attached to RDS (for additional ingress rules from new clients)."
  value       = aws_security_group.rds.id
}

output "admin_secret_arn" {
  description = "ARN of the AWS-managed Secrets Manager secret for the RDS admin password (KAI-220)"
  value       = try(aws_db_instance.primary.master_user_secret[0].secret_arn, null)
  sensitive   = true
}

output "parameter_group_name" {
  description = "Custom parameter group name (consumers may apply hot reloads via this name)."
  value       = aws_db_parameter_group.main.name
}

output "monitoring_role_arn" {
  description = "Enhanced monitoring IAM role ARN."
  value       = aws_iam_role.enhanced_monitoring.arn
}
