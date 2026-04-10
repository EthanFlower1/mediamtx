# KAI-355: Credential vault for integrator mobile signing credentials.
#
# Stores Apple/Google signing credentials in AWS Secrets Manager with a
# customer-managed KMS key. Per-tenant isolation is enforced at the
# application layer via path prefixes: kaivue/{tenant_id}/mobile/{type}.

resource "aws_kms_key" "credential_vault" {
  description             = "Kaivue credential vault encryption key"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = merge(var.tags, {
    Name    = "kaivue-credential-vault"
    Purpose = "mobile-signing-credentials"
  })
}

resource "aws_kms_alias" "credential_vault" {
  name          = "alias/kaivue-credential-vault"
  target_key_id = aws_kms_key.credential_vault.key_id
}

# IAM policy allowing the control plane service to manage secrets under
# the kaivue/ prefix.
data "aws_iam_policy_document" "credential_vault_access" {
  statement {
    sid    = "SecretsManagerCRUD"
    effect = "Allow"
    actions = [
      "secretsmanager:CreateSecret",
      "secretsmanager:GetSecretValue",
      "secretsmanager:PutSecretValue",
      "secretsmanager:DeleteSecret",
      "secretsmanager:ListSecrets",
      "secretsmanager:DescribeSecret",
      "secretsmanager:UpdateSecret",
    ]
    resources = [
      "arn:aws:secretsmanager:${var.region}:${var.account_id}:secret:kaivue/*",
    ]
  }

  statement {
    sid    = "KMSDecrypt"
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:Encrypt",
      "kms:GenerateDataKey",
      "kms:DescribeKey",
    ]
    resources = [aws_kms_key.credential_vault.arn]
  }
}

resource "aws_iam_policy" "credential_vault_access" {
  name        = "kaivue-credential-vault-access"
  description = "Allow control plane to manage integrator signing credentials"
  policy      = data.aws_iam_policy_document.credential_vault_access.json

  tags = var.tags
}
