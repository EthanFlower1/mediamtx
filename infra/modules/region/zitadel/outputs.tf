# KAI-220: Zitadel deployment outputs.
# These are consumed by KAI-221 (bootstrap) and the Go adapter config.

output "namespace" {
  description = "Kubernetes namespace where Zitadel is deployed"
  value       = kubernetes_namespace.zitadel.metadata[0].name
}

output "external_domain" {
  description = "Public domain for Zitadel API and console"
  value       = var.external_domain
}

output "internal_endpoint" {
  description = "Cluster-internal gRPC endpoint for Zitadel (for Go adapter)"
  value       = "zitadel.${kubernetes_namespace.zitadel.metadata[0].name}.svc.cluster.local:8080"
}

output "masterkey_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the Zitadel masterkey"
  value       = var.masterkey_secret_arn
  sensitive   = true
}

output "helm_release_name" {
  description = "Name of the Helm release"
  value       = helm_release.zitadel.name
}

output "helm_release_version" {
  description = "Deployed Helm chart version"
  value       = helm_release.zitadel.version
}

output "secrets_read_policy_arn" {
  description = "ARN of the IAM policy for Zitadel IRSA secret access"
  value       = aws_iam_policy.zitadel_secrets_read.arn
}
