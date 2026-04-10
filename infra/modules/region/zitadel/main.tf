# KAI-220: Deploy multi-tenant Zitadel to EKS.
#
# This module deploys Zitadel as a Helm release on the EKS cluster
# provisioned by KAI-215. It:
#   1. Creates a dedicated Kubernetes namespace
#   2. Provisions a Zitadel masterkey in Secrets Manager (or uses existing)
#   3. Syncs secrets (masterkey + DB credentials) into Kubernetes
#   4. Deploys the official zitadel/zitadel Helm chart
#   5. Configures external-dns + ALB ingress for the public domain
#
# Seam #3 enforcement: this module handles deployment infrastructure only.
# The Go adapter in internal/shared/auth/zitadel/ is the sole consumer of
# Zitadel APIs at the application layer.

locals {
  name_prefix = "kaivue-${var.environment}-${var.region}"
  labels = {
    "app.kubernetes.io/name"       = "zitadel"
    "app.kubernetes.io/part-of"    = "kaivue"
    "app.kubernetes.io/managed-by" = "terraform"
    "kaivue.io/environment"        = var.environment
    "kaivue.io/region"             = var.region
  }
}

# --------------------------------------------------------------------------
# Namespace
# --------------------------------------------------------------------------

resource "kubernetes_namespace" "zitadel" {
  metadata {
    name   = var.namespace
    labels = local.labels
  }
}

# --------------------------------------------------------------------------
# Masterkey — Zitadel uses a 32-byte symmetric key for encrypting secrets
# in the database. We store it in AWS Secrets Manager and sync it into K8s
# via a Kubernetes Secret.
# --------------------------------------------------------------------------

data "aws_secretsmanager_secret_version" "masterkey" {
  secret_id = var.masterkey_secret_arn
}

resource "kubernetes_secret" "masterkey" {
  metadata {
    name      = "zitadel-masterkey"
    namespace = kubernetes_namespace.zitadel.metadata[0].name
    labels    = local.labels
  }

  data = {
    masterkey = data.aws_secretsmanager_secret_version.masterkey.secret_string
  }

  type = "Opaque"
}

# --------------------------------------------------------------------------
# Database credentials — pulled from the RDS admin secret (KAI-216).
# --------------------------------------------------------------------------

data "aws_secretsmanager_secret_version" "db_admin" {
  secret_id = var.db_admin_secret_arn
}

locals {
  db_creds = jsondecode(data.aws_secretsmanager_secret_version.db_admin.secret_string)
}

resource "kubernetes_secret" "db_credentials" {
  metadata {
    name      = "zitadel-db-credentials"
    namespace = kubernetes_namespace.zitadel.metadata[0].name
    labels    = local.labels
  }

  data = {
    username = local.db_creds["username"]
    password = local.db_creds["password"]
  }

  type = "Opaque"
}

# --------------------------------------------------------------------------
# Helm release — official zitadel/zitadel chart
# --------------------------------------------------------------------------

resource "helm_release" "zitadel" {
  name       = "zitadel"
  namespace  = kubernetes_namespace.zitadel.metadata[0].name
  repository = "https://charts.zitadel.com"
  chart      = "zitadel"
  version    = var.chart_version
  timeout    = 600

  # Wait for all pods to become ready before marking the release as
  # successful. This is critical because Zitadel runs database migrations
  # on first boot, and a premature "success" would break downstream
  # consumers (KAI-221 bootstrap).
  wait = true

  values = [yamlencode({
    replicaCount = var.replica_count

    image = {
      tag = var.zitadel_image_tag
    }

    zitadel = {
      # Masterkey from K8s secret
      masterkeySecretName = kubernetes_secret.masterkey.metadata[0].name

      # External domain configuration
      configmapConfig = {
        ExternalDomain  = var.external_domain
        ExternalPort    = 443
        ExternalSecure  = true
        TLS = {
          Enabled = false # TLS terminates at the ALB, not in Zitadel
        }
        Log = {
          Level  = var.log_level
          Formatter = {
            Format = "json"
          }
        }
        Database = {
          Postgres = {
            Host     = var.db_host
            Port     = var.db_port
            Database = var.db_name
            MaxOpenConns = 25
            MaxIdleConns = 10
            MaxConnLifetime = "1h"
            MaxConnIdleTime = "5m"
            User = {
              SSL = {
                Mode = var.db_ssl_mode
              }
            }
            Admin = {
              SSL = {
                Mode = var.db_ssl_mode
              }
            }
          }
        }
        # Multi-tenant: allow runtime org creation via API
        DefaultInstance = {
          Org = {
            Human = {
              # KAI-221 will bootstrap the platform admin via the Go adapter.
              # The chart's default first-user provisioning is disabled.
              UserName  = ""
              FirstName = ""
              LastName  = ""
            }
          }
        }
      }

      # Database credentials from K8s secret
      dbSslCaCrt          = ""
      dbSslAdminCrt       = ""
      dbSslAdminKey       = ""
      secretConfig = {
        Database = {
          Postgres = {
            User = {
              Username = local.db_creds["username"]
              Password = local.db_creds["password"]
            }
            Admin = {
              Username = local.db_creds["username"]
              Password = local.db_creds["password"]
            }
          }
        }
      }
    }

    resources = {
      requests = {
        cpu    = var.cpu_request
        memory = var.memory_request
      }
      limits = {
        cpu    = var.cpu_limit
        memory = var.memory_limit
      }
    }

    # Pod disruption budget — keep at least 1 pod during rolling updates
    podDisruptionBudget = {
      enabled      = var.replica_count > 1
      minAvailable = 1
    }

    # Ingress via AWS ALB
    ingress = {
      enabled   = true
      className = var.ingress_class
      annotations = {
        "alb.ingress.kubernetes.io/scheme"           = "internet-facing"
        "alb.ingress.kubernetes.io/target-type"       = "ip"
        "alb.ingress.kubernetes.io/listen-ports"      = "[{\"HTTPS\":443}]"
        "alb.ingress.kubernetes.io/certificate-arn"   = "" # Filled by ACM discovery or cert-manager
        "alb.ingress.kubernetes.io/healthcheck-path"  = "/debug/healthz"
        "alb.ingress.kubernetes.io/healthcheck-port"  = "8080"
        "external-dns.alpha.kubernetes.io/hostname"   = var.external_domain
      }
      hosts = [
        {
          host = var.external_domain
          paths = [
            {
              path     = "/"
              pathType = "Prefix"
            }
          ]
        }
      ]
      tls = [
        {
          secretName = var.tls_secret_name
          hosts      = [var.external_domain]
        }
      ]
    }

    # Service monitor for Prometheus (KAI-422)
    metrics = {
      enabled = true
      serviceMonitor = {
        enabled = true
        labels = {
          release = "kube-prometheus-stack"
        }
      }
    }

    # Init container runs DB migrations before the main pod starts.
    # This ensures clean schema state on upgrades.
    initJob = {
      enabled = true
    }
  })]

  depends_on = [
    kubernetes_namespace.zitadel,
    kubernetes_secret.masterkey,
    kubernetes_secret.db_credentials,
  ]
}

# --------------------------------------------------------------------------
# Service account for Zitadel IRSA (IAM Roles for Service Accounts).
# Allows Zitadel pods to read Secrets Manager secrets if needed.
# --------------------------------------------------------------------------

resource "aws_iam_policy" "zitadel_secrets_read" {
  name        = "${local.name_prefix}-zitadel-secrets-read"
  description = "Allow Zitadel pods to read their own Secrets Manager secrets"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = [
          var.masterkey_secret_arn,
          var.db_admin_secret_arn
        ]
      }
    ]
  })

  tags = var.tags
}
