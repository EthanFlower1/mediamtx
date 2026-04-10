# KAI-215: EKS cluster variables.

variable "region" {
  description = "AWS region"
  type        = string
}

variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "cluster_version" {
  description = "Kubernetes version"
  type        = string
  default     = "1.29"

  validation {
    condition     = can(regex("^1\\.(2[89]|[3-9][0-9])$", var.cluster_version))
    error_message = "cluster_version must be 1.28 or higher"
  }
}

variable "vpc_id" {
  description = "VPC ID from the vpc sub-module"
  type        = string
}

variable "subnet_ids" {
  description = "Private subnet IDs for EKS worker nodes"
  type        = list(string)
}

variable "kms_key_arn" {
  description = "KMS key ARN for EKS secrets encryption"
  type        = string
}

variable "endpoint_public_access" {
  description = "Whether the EKS API server endpoint is publicly accessible"
  type        = bool
  default     = true
}

# --- System node group ---

variable "system_node_instance_types" {
  description = "Instance types for the system node group"
  type        = list(string)
  default     = ["m6i.large", "m6a.large"]
}

variable "system_node_desired" {
  description = "Desired number of system nodes"
  type        = number
  default     = 2
}

variable "system_node_min" {
  description = "Minimum number of system nodes"
  type        = number
  default     = 2
}

variable "system_node_max" {
  description = "Maximum number of system nodes"
  type        = number
  default     = 5
}

# --- Workload node group ---

variable "workload_node_instance_types" {
  description = "Instance types for the workload node group"
  type        = list(string)
  default     = ["m6i.xlarge", "m6a.xlarge"]
}

variable "workload_node_desired" {
  description = "Desired number of workload nodes"
  type        = number
  default     = 2
}

variable "workload_node_min" {
  description = "Minimum number of workload nodes"
  type        = number
  default     = 1
}

variable "workload_node_max" {
  description = "Maximum number of workload nodes"
  type        = number
  default     = 20
}

# --- IAM role ARNs for aws-auth ---

variable "eks_admin_role_arn" {
  description = "IAM role ARN for EKS admin access (from global IAM module)"
  type        = string
}

variable "ci_role_arn" {
  description = "IAM role ARN for CI/CD access (from global IAM module)"
  type        = string
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
