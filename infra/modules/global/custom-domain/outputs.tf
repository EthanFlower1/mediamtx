output "verify_zone_id" {
  description = "Route53 zone ID for the verification domain"
  value       = aws_route53_zone.verify.zone_id
}

output "verify_zone_name_servers" {
  description = "Name servers for the verification zone (delegate from parent)"
  value       = aws_route53_zone.verify.name_servers
}

output "verify_certificate_arn" {
  description = "ACM certificate ARN for the verification domain"
  value       = aws_acm_certificate.verify.arn
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution ID for custom domains"
  value       = aws_cloudfront_distribution.custom_domains.id
}

output "cloudfront_domain_name" {
  description = "CloudFront distribution domain name"
  value       = aws_cloudfront_distribution.custom_domains.domain_name
}
