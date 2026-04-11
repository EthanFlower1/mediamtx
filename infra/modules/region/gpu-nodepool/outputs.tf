# KAI-277: GPU node pool outputs.

output "gpu_node_group_name" {
  description = "EKS GPU node group name"
  value       = aws_eks_node_group.gpu_inference.node_group_name
}

output "gpu_node_group_arn" {
  description = "EKS GPU node group ARN"
  value       = aws_eks_node_group.gpu_inference.arn
}

output "gpu_node_group_status" {
  description = "EKS GPU node group status"
  value       = aws_eks_node_group.gpu_inference.status
}
