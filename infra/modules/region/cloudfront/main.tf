# KAI-231: CloudFront distribution stub.
# Static assets + JWKS endpoint are served via CloudFront.
# The JWKS cache (for Recorder-local JWT verification, KAI-256)
# must have a short TTL (300s) so key rotation propagates quickly.

resource "aws_cloudfront_distribution" "main" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "kaivue-${var.environment}-${var.region}"
  price_class         = "PriceClass_100"
  retain_on_delete    = false
  wait_for_deployment = false

  origin {
    domain_name = var.alb_dns
    origin_id   = "alb-${var.region}"

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    allowed_methods        = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "alb-${var.region}"
    viewer_protocol_policy = "redirect-to-https"

    forwarded_values {
      query_string = true
      headers      = ["Host", "Authorization", "X-Kaivue-Tenant-ID"]
      cookies {
        forward = "none"
      }
    }

    min_ttl     = 0
    default_ttl = 0
    max_ttl     = 0
  }

  # Short-TTL cache behavior for /.well-known/jwks.json (KAI-256)
  ordered_cache_behavior {
    path_pattern           = "/.well-known/jwks.json"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "alb-${var.region}"
    viewer_protocol_policy = "redirect-to-https"

    forwarded_values {
      query_string = false
      cookies {
        forward = "none"
      }
    }

    min_ttl     = 60
    default_ttl = 300
    max_ttl     = 300
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = var.tags
}
