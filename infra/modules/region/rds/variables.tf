# KAI-216: RDS Postgres + pgvector + read replica module variables.

variable "region" {
  description = "AWS region (e.g. us-east-2)."
  type        = string
}

variable "environment" {
  description = "Deployment environment (dev|staging|prod)."
  type        = string
}

variable "db_instance_class" {
  description = "RDS instance class for the primary writer."
  type        = string
}

variable "vpc_id" {
  description = "VPC ID where the DB subnet group lives."
  type        = string
}

variable "subnet_ids" {
  description = "Private subnet IDs for the DB subnet group (≥2 AZs)."
  type        = list(string)
}

variable "kms_key_arn" {
  description = "KMS key ARN for storage, Performance Insights, and Secrets Manager encryption."
  type        = string
}

variable "allowed_security_group_ids" {
  description = "Security group IDs allowed to talk to RDS on 5432 (typically the EKS node SG from KAI-215)."
  type        = list(string)
  default     = []
}

variable "allocated_storage_gb" {
  description = "Initial allocated storage in GiB. RDS storage autoscaling can grow this up to max_allocated_storage_gb."
  type        = number
  default     = 100
}

variable "max_allocated_storage_gb" {
  description = "Storage autoscaling ceiling in GiB."
  type        = number
  default     = 1000
}

variable "multi_az" {
  description = "Provision the primary as Multi-AZ. Required in prod for SOC 2 RPO/RTO."
  type        = bool
  default     = true
}

variable "backup_retention_days" {
  description = "Days of automated backups retained."
  type        = number
  default     = 14
}

variable "performance_insights_retention_days" {
  description = "Performance Insights retention. 7 = free tier, 731 = 2 years (paid)."
  type        = number
  default     = 7

  validation {
    condition     = contains([7, 31, 62, 93, 124, 155, 186, 217, 248, 279, 310, 341, 372, 731], var.performance_insights_retention_days)
    error_message = "performance_insights_retention_days must be 7 or a multiple of 31 up to 372, or 731 (2 years)."
  }
}

variable "max_connections" {
  description = "Postgres max_connections ceiling. Sized for the connection pool budget across the cloud control plane."
  type        = number
  default     = 500
}

variable "statement_timeout_ms" {
  description = "Postgres statement_timeout in milliseconds (last-resort runaway query kill)."
  type        = number
  default     = 60000
}

variable "enable_read_replica" {
  description = "Provision an in-region read replica. Off by default in v1; flip on for active-active v1.x expansion."
  type        = bool
  default     = false
}

variable "replica_instance_class" {
  description = "Instance class for the read replica. Defaults to the primary class if empty."
  type        = string
  default     = ""
}

variable "alarm_sns_topic_arns" {
  description = "SNS topic ARNs for CloudWatch alarm actions. Empty disables alarm notifications (alarms still fire in the console)."
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Resource tags merged into every child resource."
  type        = map(string)
  default     = {}
}
