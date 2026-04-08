# KAI-231: Cross-account IAM roles stub.
# KAI-232 (CI/CD) will attach the deployment policy to ci_role.
# KAI-224 (cross-tenant access) will define integrator federation roles here
# with the integrator:/federation: subject prefix conventions (Casbin).

resource "aws_iam_role" "ci" {
  name = "kaivue-${var.environment}-ci-deploy"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        # GitHub Actions OIDC — replace thumbprint list and URL in KAI-232
        Federated = "arn:aws:iam::${var.aws_account_id}:oidc-provider/token.actions.githubusercontent.com"
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringLike = {
          "token.actions.githubusercontent.com:sub" = "repo:kaivue/*:*"
        }
      }
    }]
  })

  tags = var.tags
}

resource "aws_iam_role" "readonly" {
  name = "kaivue-${var.environment}-readonly"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = "arn:aws:iam::${var.aws_account_id}:root" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "readonly" {
  role       = aws_iam_role.readonly.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}
