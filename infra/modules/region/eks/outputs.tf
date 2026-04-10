# KAI-215: EKS cluster outputs.

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

output "node_role_arn" {
  description = "IAM role ARN used by EKS worker nodes"
  value       = aws_iam_role.node.arn
}

output "oidc_provider_arn" {
  description = "OIDC provider ARN for IRSA"
  value       = aws_iam_openid_connect_provider.eks.arn
}

output "oidc_provider_url" {
  description = "OIDC provider URL (without https:// prefix)"
  value       = replace(aws_iam_openid_connect_provider.eks.url, "https://", "")
}

output "karpenter_role_arn" {
  description = "IAM role ARN for Karpenter controller (IRSA)"
  value       = aws_iam_role.karpenter.arn
}

output "karpenter_instance_profile_name" {
  description = "Instance profile name for Karpenter-provisioned nodes"
  value       = aws_iam_instance_profile.karpenter_node.name
}

output "alb_controller_role_arn" {
  description = "IAM role ARN for AWS Load Balancer Controller (IRSA)"
  value       = aws_iam_role.alb_controller.arn
}

output "external_dns_role_arn" {
  description = "IAM role ARN for External DNS (IRSA)"
  value       = aws_iam_role.external_dns.arn
}

output "aws_auth_roles" {
  description = "aws-auth ConfigMap role entries (apply via Helm/ArgoCD)"
  value       = local.aws_auth_roles
}

output "node_security_group_id" {
  description = "Security group ID for EKS worker nodes"
  value       = aws_security_group.node.id
}

output "cluster_security_group_id" {
  description = "Security group ID for EKS cluster control plane"
  value       = aws_security_group.cluster.id
}
