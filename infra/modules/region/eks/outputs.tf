output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.main.name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint"
  value       = aws_eks_cluster.main.endpoint
  sensitive   = true
}

output "cluster_ca_data" {
  description = "EKS cluster CA certificate data"
  value       = aws_eks_cluster.main.certificate_authority[0].data
  sensitive   = true
}

output "cluster_iam_role_arn" {
  description = "IAM role ARN used by the EKS control plane"
  value       = aws_iam_role.cluster.arn
}
