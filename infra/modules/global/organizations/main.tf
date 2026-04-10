# KAI-214: AWS Organizations — OU hierarchy + Service Control Policies.
# This is the management-account singleton. Only one aws_organizations_organization
# resource may exist per AWS org. Child accounts are referenced as data sources
# or created via aws_organizations_account in environment-specific root modules.

resource "aws_organizations_organization" "org" {
  aws_service_access_principals = [
    "cloudtrail.amazonaws.com",
    "config.amazonaws.com",
    "guardduty.amazonaws.com",
    "sso.amazonaws.com",
    "ram.amazonaws.com",
  ]

  enabled_policy_types = [
    "SERVICE_CONTROL_POLICY",
    "TAG_POLICY",
  ]

  feature_set = "ALL"
}

# ---------------------------------------------------------------------------
# Organizational Units
# ---------------------------------------------------------------------------

resource "aws_organizations_organizational_unit" "security" {
  name      = "Security"
  parent_id = aws_organizations_organization.org.roots[0].id
  tags      = var.tags
}

resource "aws_organizations_organizational_unit" "infrastructure" {
  name      = "Infrastructure"
  parent_id = aws_organizations_organization.org.roots[0].id
  tags      = var.tags
}

resource "aws_organizations_organizational_unit" "workloads" {
  name      = "Workloads"
  parent_id = aws_organizations_organization.org.roots[0].id
  tags      = var.tags
}

resource "aws_organizations_organizational_unit" "workloads_production" {
  name      = "Production"
  parent_id = aws_organizations_organizational_unit.workloads.id
  tags      = var.tags
}

resource "aws_organizations_organizational_unit" "workloads_staging" {
  name      = "Staging"
  parent_id = aws_organizations_organizational_unit.workloads.id
  tags      = var.tags
}

resource "aws_organizations_organizational_unit" "sandbox" {
  name      = "Sandbox"
  parent_id = aws_organizations_organization.org.roots[0].id
  tags      = var.tags
}

# ---------------------------------------------------------------------------
# Service Control Policies
# ---------------------------------------------------------------------------

# SCP 1: Region restriction — workload accounts can only operate in approved regions.
resource "aws_organizations_policy" "region_restriction" {
  name        = "kaivue-region-restriction"
  description = "Deny API calls outside approved regions (except global services)"
  type        = "SERVICE_CONTROL_POLICY"
  tags        = var.tags

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "DenyUnapprovedRegions"
      Effect = "Deny"
      NotAction = [
        "a4b:*",
        "budgets:*",
        "ce:*",
        "chime:*",
        "cloudfront:*",
        "cur:*",
        "globalaccelerator:*",
        "health:*",
        "iam:*",
        "importexport:*",
        "organizations:*",
        "route53:*",
        "route53domains:*",
        "s3:GetBucketLocation",
        "s3:ListAllMyBuckets",
        "sts:GetCallerIdentity",
        "sts:GetSessionToken",
        "sts:DecodeAuthorizationMessage",
        "support:*",
        "trustedadvisor:*",
        "waf:*",
        "wafv2:*",
      ]
      Resource = "*"
      Condition = {
        StringNotEquals = {
          "aws:RequestedRegion" = var.approved_regions
        }
      }
    }]
  })
}

# SCP 2: Deny root user actions in child accounts.
resource "aws_organizations_policy" "deny_root" {
  name        = "kaivue-deny-root-user"
  description = "Deny all actions by the root user in workload accounts"
  type        = "SERVICE_CONTROL_POLICY"
  tags        = var.tags

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "DenyRootUser"
      Effect   = "Deny"
      Action   = "*"
      Resource = "*"
      Condition = {
        StringLike = {
          "aws:PrincipalArn" = "arn:aws:iam::*:root"
        }
      }
    }]
  })
}

# SCP 3: Prevent accounts from leaving the organization.
resource "aws_organizations_policy" "deny_leave_org" {
  name        = "kaivue-deny-leave-org"
  description = "Prevent workload accounts from leaving the organization"
  type        = "SERVICE_CONTROL_POLICY"
  tags        = var.tags

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "DenyLeaveOrganization"
      Effect   = "Deny"
      Action   = "organizations:LeaveOrganization"
      Resource = "*"
    }]
  })
}

# SCP 4: Protect security services — prevent disabling CloudTrail, GuardDuty, Config.
resource "aws_organizations_policy" "protect_security_services" {
  name        = "kaivue-protect-security-services"
  description = "Deny disabling CloudTrail, GuardDuty, and AWS Config in workload accounts"
  type        = "SERVICE_CONTROL_POLICY"
  tags        = var.tags

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "DenyCloudTrailDisable"
        Effect = "Deny"
        Action = [
          "cloudtrail:DeleteTrail",
          "cloudtrail:StopLogging",
          "cloudtrail:UpdateTrail",
        ]
        Resource = "*"
      },
      {
        Sid    = "DenyGuardDutyDisable"
        Effect = "Deny"
        Action = [
          "guardduty:DeleteDetector",
          "guardduty:DisassociateFromMasterAccount",
          "guardduty:UpdateDetector",
        ]
        Resource = "*"
      },
      {
        Sid    = "DenyConfigDisable"
        Effect = "Deny"
        Action = [
          "config:DeleteConfigurationRecorder",
          "config:DeleteDeliveryChannel",
          "config:StopConfigurationRecorder",
        ]
        Resource = "*"
      },
    ]
  })
}

# ---------------------------------------------------------------------------
# SCP Attachments — apply to workload OUs (production + staging)
# Security and Infrastructure OUs are left with full SCP access by design;
# the management account itself cannot be restricted by SCPs.
# ---------------------------------------------------------------------------

resource "aws_organizations_policy_attachment" "region_restriction_production" {
  policy_id = aws_organizations_policy.region_restriction.id
  target_id = aws_organizations_organizational_unit.workloads_production.id
}

resource "aws_organizations_policy_attachment" "region_restriction_staging" {
  policy_id = aws_organizations_policy.region_restriction.id
  target_id = aws_organizations_organizational_unit.workloads_staging.id
}

resource "aws_organizations_policy_attachment" "deny_root_production" {
  policy_id = aws_organizations_policy.deny_root.id
  target_id = aws_organizations_organizational_unit.workloads_production.id
}

resource "aws_organizations_policy_attachment" "deny_root_staging" {
  policy_id = aws_organizations_policy.deny_root.id
  target_id = aws_organizations_organizational_unit.workloads_staging.id
}

resource "aws_organizations_policy_attachment" "deny_leave_production" {
  policy_id = aws_organizations_policy.deny_leave_org.id
  target_id = aws_organizations_organizational_unit.workloads_production.id
}

resource "aws_organizations_policy_attachment" "deny_leave_staging" {
  policy_id = aws_organizations_policy.deny_leave_org.id
  target_id = aws_organizations_organizational_unit.workloads_staging.id
}

resource "aws_organizations_policy_attachment" "protect_security_production" {
  policy_id = aws_organizations_policy.protect_security_services.id
  target_id = aws_organizations_organizational_unit.workloads_production.id
}

resource "aws_organizations_policy_attachment" "protect_security_staging" {
  policy_id = aws_organizations_policy.protect_security_services.id
  target_id = aws_organizations_organizational_unit.workloads_staging.id
}

# Sandbox gets region restriction + deny-leave but NOT deny-root or protect-security,
# allowing sandbox accounts to have broader access for experimentation.
resource "aws_organizations_policy_attachment" "region_restriction_sandbox" {
  policy_id = aws_organizations_policy.region_restriction.id
  target_id = aws_organizations_organizational_unit.sandbox.id
}

resource "aws_organizations_policy_attachment" "deny_leave_sandbox" {
  policy_id = aws_organizations_policy.deny_leave_org.id
  target_id = aws_organizations_organizational_unit.sandbox.id
}
