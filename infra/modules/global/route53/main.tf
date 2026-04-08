# KAI-231: Route53 global DNS sub-module.
# Each active region gets a per-region API subdomain:
#   https://us-east-2.api.yourbrand.com/v1/...
# Adding eu-west-1 is: add the entry to active_regions and supply the ALB outputs.
# KAI-230 (region routing) will add latency-based routing policies here.

resource "aws_route53_zone" "main" {
  name    = var.root_domain
  comment = "kaivue-${var.environment} managed by Terraform (KAI-231)"
  tags    = var.tags
}

# Per-region API subdomains: <region>.api.<root_domain>
resource "aws_route53_record" "region_api" {
  for_each = toset(var.active_regions)

  zone_id = aws_route53_zone.main.zone_id
  name    = "${each.key}.api.${var.root_domain}"
  type    = "A"

  dynamic "alias" {
    for_each = lookup(var.region_alb_dns, each.key, "") != "" ? [1] : []
    content {
      name                   = var.region_alb_dns[each.key]
      zone_id                = var.region_alb_zone_id[each.key]
      evaluate_target_health = true
    }
  }

  # Placeholder when ALB outputs are not yet wired (initial bootstrap)
  dynamic "alias" {
    for_each = lookup(var.region_alb_dns, each.key, "") == "" ? [1] : []
    content {
      name                   = "placeholder.${each.key}.elb.amazonaws.com"
      zone_id                = "Z3AADJGX6KTTL2" # us-east-2 ELB zone ID
      evaluate_target_health = false
    }
  }
}
