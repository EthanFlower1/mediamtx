# KAI-356: Infrastructure for custom domain CNAME verification and TLS provisioning.

# Route53 hosted zone for the verification target domain.
# Integrators create CNAME records pointing to this zone:
#   _acme-challenge.cameras.acme.com -> verify.kaivue.io
resource "aws_route53_zone" "verify" {
  name    = "verify.${var.base_domain}"
  comment = "Kaivue custom domain CNAME verification zone"

  tags = merge(var.tags, {
    Name    = "kaivue-verify-zone"
    Purpose = "custom-domain-verification"
  })
}

# ACM wildcard certificate for the verification domain itself.
resource "aws_acm_certificate" "verify" {
  domain_name       = "verify.${var.base_domain}"
  validation_method = "DNS"

  tags = merge(var.tags, {
    Name = "kaivue-verify-cert"
  })

  lifecycle {
    create_before_destroy = true
  }
}

# DNS validation record for the ACM certificate.
resource "aws_route53_record" "verify_cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.verify.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = aws_route53_zone.verify.zone_id
}

resource "aws_acm_certificate_validation" "verify" {
  certificate_arn         = aws_acm_certificate.verify.arn
  validation_record_fqdns = [for record in aws_route53_record.verify_cert_validation : record.fqdn]
}

# CloudFront distribution for serving custom domains.
# Each integrator's custom domain is added as an alternate domain name.
# Certificate attachment happens at the application layer via API calls.
resource "aws_cloudfront_distribution" "custom_domains" {
  enabled         = true
  comment         = "Kaivue custom domain distribution"
  is_ipv6_enabled = true
  aliases         = [] # Managed dynamically by the application

  origin {
    domain_name = var.origin_domain
    origin_id   = "kaivue-app"

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    allowed_methods        = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "kaivue-app"
    viewer_protocol_policy = "redirect-to-https"

    forwarded_values {
      query_string = true
      headers      = ["Host", "Authorization"]
      cookies {
        forward = "all"
      }
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = merge(var.tags, {
    Name    = "kaivue-custom-domains"
    Purpose = "integrator-white-label"
  })
}
