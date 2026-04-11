# KAI-277: GPU node pool variables.

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
}

variable "node_role_arn" {
  description = "IAM role ARN for EKS worker nodes"
  type        = string
}

variable "subnet_ids" {
  description = "Private subnet IDs for GPU nodes"
  type        = list(string)
}

variable "gpu_instance_types" {
  description = "Instance types for the GPU node group"
  type        = list(string)
  default     = ["g5.2xlarge", "g5.4xlarge"]
}

variable "gpu_node_desired" {
  description = "Desired number of GPU nodes"
  type        = number
  default     = 0
}

variable "gpu_node_min" {
  description = "Minimum number of GPU nodes (0 enables scale-to-zero)"
  type        = number
  default     = 0
}

variable "gpu_node_max" {
  description = "Maximum number of GPU nodes"
  type        = number
  default     = 8
}

variable "nvidia_device_plugin_version" {
  description = "NVIDIA device plugin Helm chart version"
  type        = string
  default     = "0.15.0"
}

variable "keda_version" {
  description = "KEDA Helm chart version"
  type        = string
  default     = "2.14.0"
}

variable "tags" {
  description = "Resource tags"
  type        = map(string)
  default     = {}
}
