# KAI-231: EKS sub-module stub.
# KAI-215 will replace this stub with managed node groups, Karpenter,
# IRSA roles, aws-auth ConfigMap, Flux/ArgoCD bootstrap, and
# kube-prometheus-stack. Do not add real node groups here.

resource "aws_iam_role" "cluster" {
  name = "kaivue-${var.environment}-${var.region}-eks-cluster"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "cluster_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.cluster.name
}

resource "aws_eks_cluster" "main" {
  name     = "kaivue-${var.environment}-${var.region}"
  role_arn = aws_iam_role.cluster.arn
  version  = var.cluster_version

  vpc_config {
    subnet_ids = var.subnet_ids
  }

  encryption_config {
    resources = ["secrets"]
    provider {
      key_arn = var.kms_key_arn
    }
  }

  tags = var.tags

  depends_on = [aws_iam_role_policy_attachment.cluster_policy]
}
