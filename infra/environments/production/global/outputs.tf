output "hosted_zone_id" {
  value = module.global.hosted_zone_id
}

output "name_servers" {
  description = "Set these at your registrar after initial apply"
  value       = module.global.api_fqdn
}

output "ci_role_arn" {
  value = module.global.ci_role_arn
}
