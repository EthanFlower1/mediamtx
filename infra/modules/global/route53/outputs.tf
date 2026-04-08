output "hosted_zone_id" {
  description = "Route53 hosted zone ID"
  value       = aws_route53_zone.main.zone_id
}

output "name_servers" {
  description = "Route53 name servers (set these at your registrar)"
  value       = aws_route53_zone.main.name_servers
}

output "api_fqdns" {
  description = "Map of region -> API FQDN"
  value = {
    for region in var.active_regions :
    region => "${region}.api.${var.root_domain}"
  }
}
