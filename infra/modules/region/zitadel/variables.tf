# KAI-220: Zitadel deployment variables.

variable "environment" {
  description = "Environment name (e.g. staging, production)"
  type        = string

  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment must be staging or production"
  }
}

variable "region" {
  description = "AWS region identifier (e.g. us-east-2)"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace for Zitadel"
  type        = string
  default     = "zitadel"
}

# --- Zitadel Helm chart ---

variable "chart_version" {
  description = "Zitadel Helm chart version"
  type        = string
  default     = "8.5.0"
}

variable "zitadel_image_tag" {
  description = "Zitadel container image tag (e.g. v2.62.0)"
  type        = string
  default     = "v2.62.0"
}

variable "replica_count" {
  description = "Number of Zitadel pods"
  type        = number
  default     = 2

  validation {
    condition     = var.replica_count >= 1 && var.replica_count <= 10
    error_message = "replica_count must be between 1 and 10"
  }
}

# --- Database (RDS Postgres) ---

variable "db_host" {
  description = "RDS Postgres writer endpoint for Zitadel's database"
  type        = string
}

variable "db_port" {
  description = "RDS Postgres port"
  type        = number
  default     = 5432
}

variable "db_name" {
  description = "Database name for Zitadel"
  type        = string
  default     = "zitadel"
}

variable "db_ssl_mode" {
  description = "Postgres SSL mode (require for RDS)"
  type        = string
  default     = "require"
}

variable "db_admin_secret_arn" {
  description = "ARN of the Secrets Manager secret containing the RDS admin credentials (JSON: {username, password})"
  type        = string
}

# --- Networking ---

variable "external_domain" {
  description = "Public domain for Zitadel (e.g. auth.kaivue.com)"
  type        = string
}

variable "tls_secret_name" {
  description = "Kubernetes TLS secret name for the Zitadel ingress (cert-manager managed)"
  type        = string
  default     = "zitadel-tls"
}

variable "ingress_class" {
  description = "Kubernetes IngressClass name"
  type        = string
  default     = "alb"
}

# --- Service account key ---

variable "masterkey_secret_arn" {
  description = "ARN of the Secrets Manager secret containing the Zitadel masterkey (32-byte hex)"
  type        = string
}

# --- Resource requests ---

variable "cpu_request" {
  description = "CPU request per Zitadel pod"
  type        = string
  default     = "250m"
}

variable "memory_request" {
  description = "Memory request per Zitadel pod"
  type        = string
  default     = "512Mi"
}

variable "cpu_limit" {
  description = "CPU limit per Zitadel pod"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit per Zitadel pod"
  type        = string
  default     = "1Gi"
}

# --- Observability ---

variable "log_level" {
  description = "Zitadel log level"
  type        = string
  default     = "info"

  validation {
    condition     = contains(["debug", "info", "warn", "error"], var.log_level)
    error_message = "log_level must be debug, info, warn, or error"
  }
}

variable "tags" {
  description = "AWS resource tags"
  type        = map(string)
  default     = {}
}
