# Kaivue Infrastructure (KAI-231)

Terraform IaC for the Kaivue cloud control plane. Designed to make
adding region #2 an "uncomment a directory + terraform apply" operation.

## Layout

```
infra/
  modules/
    region/          # Everything one region needs (KAI-231)
      eks/           # EKS cluster stub (KAI-215)
      rds/           # RDS Postgres stub (KAI-216)
      redis/         # ElastiCache Redis stub (KAI-217)
      kms/           # KMS keys for EKS/RDS/Redis/Secrets
      vpc/           # VPC + subnets
      alb/           # ALB (KAI-230 region routing)
      cloudfront/    # CloudFront + JWKS cache (KAI-256)
    global/          # Cross-region resources
      route53/       # DNS per-region API subdomains
      organizations/ # AWS Org SCPs (KAI-214)
      iam/           # CI/CD IAM roles (KAI-232)
      stripe/        # Stripe Connect secrets (KAI-361)
  environments/
    production/regions/us-east-2/   # Active in v1
    production/global/
    staging/regions/us-east-2/
    staging/global/
    dev/regions/us-east-2/
    dev/global/
```

## Validate (no AWS credentials required)

```bash
cd infra && terraform fmt -check -recursive

cd environments/production/regions/us-east-2
terraform init -backend=false
terraform validate

cd ../../global
terraform init -backend=false
terraform validate
```

## How to add region #2 (v1.x runbook)

1. Copy `environments/production/regions/us-east-2/` to
   `environments/production/regions/eu-west-1/`.
2. Update `main.tf` in the copy: set `region = "eu-west-1"`,
   `vpc_cidr = "10.3.0.0/16"`, and `availability_zones`.
3. Add a `provider "aws" { alias = "eu-west-1"; region = "eu-west-1" }`
   block and pass it to the module.
4. Apply the new region stack:
   ```
   cd environments/production/regions/eu-west-1
   terraform init && terraform apply
   ```
5. Capture the `alb_dns_name` and `alb_zone_id` outputs.
6. In `environments/production/global/main.tf`, add `eu-west-1` to
   `active_regions` and supply the ALB outputs in `region_alb_dns` /
   `region_alb_zone_id`.
7. Apply the global stack to create the Route53 A-record for
   `eu-west-1.api.yourbrand.com`.
8. Update KAI-230 (region routing) to add the new region's weighted/latency
   routing policy.

No module code changes needed. The `modules/region/` directory is already
parameterized for any `region` value.

## Remote state

Each environment uses a separate S3 bucket + DynamoDB lock table.
Backend blocks are left as commented `# TODO: configure remote state`
placeholders. KAI-232 (CI/CD) will provision the state buckets and
uncomment the backend configs.

## Architectural constraints

- Never put region-scoped resources in `modules/global/`.
- Never branch on `environment` inside a module; use tfvars instead.
- Never hardcode `aws_account_id`; always inject via `TF_VAR_aws_account_id`.
- Never commit `.tfstate` files (enforced by `.gitignore`).
- All API endpoints are under `https://<region>.api.yourbrand.com/v1/...`
  (KAI-230).
