output "dns_name" {
  description = "ALB DNS name (used as CloudFront origin and Route53 alias target)"
  value       = aws_lb.main.dns_name
}

output "arn" {
  description = "ALB ARN"
  value       = aws_lb.main.arn
}

output "zone_id" {
  description = "ALB hosted zone ID (for Route53 alias records)"
  value       = aws_lb.main.zone_id
}

output "api_target_group_arn" {
  description = "Target group ARN for the API service"
  value       = aws_lb_target_group.api.arn
}
