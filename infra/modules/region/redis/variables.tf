# KAI-217: ElastiCache Redis variables.

variable "region" {
  description = "AWS region"
  type        = string
}

variable "environment" {
  description = "Deployment environment (dev/staging/prod)"
  type        = string
}

variable "redis_node_type" {
  description = "ElastiCache node type"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID"
  type        = string
}

variable "subnet_ids" {
  description = "Private subnet IDs for the cache subnet group"
  type        = list(string)
}

variable "kms_key_arn" {
  description = "KMS key ARN for at-rest encryption + Secrets Manager + CloudWatch log groups"
  type        = string
}

variable "allowed_security_group_ids" {
  description = "Security group IDs permitted to reach Redis on 6379 (EKS node groups)"
  type        = list(string)
  default     = []
}

variable "num_cache_clusters" {
  description = "Number of cache clusters (1 = single-node, 2+ = primary + replicas with automatic failover). Must be >= 2 for multi-AZ."
  type        = number
  default     = 2

  validation {
    condition     = var.num_cache_clusters >= 1 && var.num_cache_clusters <= 6
    error_message = "num_cache_clusters must be between 1 and 6."
  }
}

variable "maxmemory_policy" {
  description = "Redis maxmemory-policy. volatile-lru keeps session keys with TTLs but never evicts persistent config; allkeys-lru evicts anything."
  type        = string
  default     = "volatile-lru"

  validation {
    condition = contains([
      "noeviction",
      "allkeys-lru",
      "allkeys-lfu",
      "volatile-lru",
      "volatile-lfu",
      "allkeys-random",
      "volatile-random",
      "volatile-ttl",
    ], var.maxmemory_policy)
    error_message = "maxmemory_policy must be a valid Redis eviction policy."
  }
}

variable "client_idle_timeout_seconds" {
  description = "Server-side client idle timeout (seconds). 0 disables. Default 300s bounds connection count."
  type        = number
  default     = 300
}

variable "maintenance_window" {
  description = "Preferred weekly maintenance window (ddd:hh24:mi-ddd:hh24:mi, UTC)"
  type        = string
  default     = "sun:05:00-sun:07:00"
}

variable "snapshot_window" {
  description = "Daily snapshot window (hh24:mi-hh24:mi, UTC). Must not overlap maintenance_window."
  type        = string
  default     = "03:00-04:00"
}

variable "snapshot_retention_days" {
  description = "Number of days to retain automatic snapshots"
  type        = number
  default     = 7
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention for slow-log + engine-log"
  type        = number
  default     = 30
}

variable "secret_recovery_window_days" {
  description = "Secrets Manager recovery window for the auth-token secret (0 = immediate delete)"
  type        = number
  default     = 7
}

variable "cpu_alarm_threshold" {
  description = "EngineCPUUtilization % alarm threshold"
  type        = number
  default     = 75
}

variable "memory_alarm_threshold" {
  description = "DatabaseMemoryUsagePercentage alarm threshold"
  type        = number
  default     = 80
}

variable "replication_lag_alarm_seconds" {
  description = "ReplicationLag alarm threshold in seconds"
  type        = number
  default     = 5
}

variable "alarm_sns_topic_arns" {
  description = "SNS topic ARNs for alarm actions"
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
