# KAI-214: Global IAM roles for account bootstrap.
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

# ---------------------------------------------------------------------------
# EKS Admin role — assumed by platform engineers for cluster management.
# KAI-215 will add this role to the aws-auth ConfigMap.
# ---------------------------------------------------------------------------

resource "aws_iam_role" "eks_admin" {
  name = "kaivue-${var.environment}-eks-admin"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = "arn:aws:iam::${var.aws_account_id}:root" }
      Action    = "sts:AssumeRole"
      Condition = {
        BoolIfExists = {
          "aws:MultiFactorAuthPresent" = "true"
        }
      }
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "eks_admin" {
  role       = aws_iam_role.eks_admin.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# ---------------------------------------------------------------------------
# Terraform state manager role — CI assumes this for state locking.
# Scoped to S3 state bucket + DynamoDB lock table only.
# ---------------------------------------------------------------------------

resource "aws_iam_role" "terraform_state" {
  name = "kaivue-${var.environment}-terraform-state"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        AWS = aws_iam_role.ci.arn
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "terraform_state" {
  name = "terraform-state-access"
  role = aws_iam_role.terraform_state.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "S3StateAccess"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
        ]
        Resource = [
          "arn:aws:s3:::kaivue-${var.environment}-terraform-state",
          "arn:aws:s3:::kaivue-${var.environment}-terraform-state/*",
        ]
      },
      {
        Sid    = "DynamoDBLockAccess"
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:DeleteItem",
        ]
        Resource = "arn:aws:dynamodb:*:${var.aws_account_id}:table/kaivue-${var.environment}-terraform-locks"
      },
    ]
  })
}

# ---------------------------------------------------------------------------
# Break-glass role — emergency admin access with MFA + session duration limit.
# Usage is logged and alerted on via CloudTrail + EventBridge (KAI-422).
# ---------------------------------------------------------------------------

resource "aws_iam_role" "break_glass" {
  name                 = "kaivue-${var.environment}-break-glass"
  max_session_duration = 3600 # 1 hour max

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = "arn:aws:iam::${var.aws_account_id}:root" }
      Action    = "sts:AssumeRole"
      Condition = {
        BoolIfExists = {
          "aws:MultiFactorAuthPresent" = "true"
        }
      }
    }]
  })

  tags = merge(var.tags, {
    SecurityClassification = "break-glass"
    KAITicket              = "KAI-214"
  })
}

resource "aws_iam_role_policy_attachment" "break_glass" {
  role       = aws_iam_role.break_glass.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
