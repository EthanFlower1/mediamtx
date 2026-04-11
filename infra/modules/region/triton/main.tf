# KAI-277: Triton Inference Server IRSA and S3 model repository access.
# This module creates the IAM role for the Triton service account to
# pull models from S3 and the S3 bucket for the model repository.

# ---------------------------------------------------------------------------
# S3 bucket for Triton model repository
# ---------------------------------------------------------------------------

resource "aws_s3_bucket" "model_repository" {
  bucket = "kaivue-${var.environment}-${var.region}-models"

  tags = merge(var.tags, {
    "kaivue.io/component" = "ml-inference"
    "kaivue.io/ticket"    = "KAI-277"
  })
}

resource "aws_s3_bucket_versioning" "model_repository" {
  bucket = aws_s3_bucket.model_repository.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "model_repository" {
  bucket = aws_s3_bucket.model_repository.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = var.kms_key_arn
    }
  }
}

resource "aws_s3_bucket_public_access_block" "model_repository" {
  bucket = aws_s3_bucket.model_repository.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ---------------------------------------------------------------------------
# IRSA: Triton Inference Server
# ---------------------------------------------------------------------------

resource "aws_iam_role" "triton" {
  name = "kaivue-${var.environment}-${var.region}-triton"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.oidc_provider_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${var.oidc_provider_url}:aud" = "sts.amazonaws.com"
          "${var.oidc_provider_url}:sub" = "system:serviceaccount:ml-inference:triton-inference"
        }
      }
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "triton_s3" {
  name = "triton-s3-model-repository"
  role = aws_iam_role.triton.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ListModelBucket"
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:GetBucketLocation",
        ]
        Resource = aws_s3_bucket.model_repository.arn
      },
      {
        Sid    = "ReadModelObjects"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:GetObjectVersion",
        ]
        Resource = "${aws_s3_bucket.model_repository.arn}/*"
      },
      {
        Sid    = "KMSDecrypt"
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey",
        ]
        Resource = var.kms_key_arn
      },
    ]
  })
}
