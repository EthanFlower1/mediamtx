output "organization_id" {
  description = "AWS Organizations organization ID"
  value       = aws_organizations_organization.org.id
}

output "organization_root_id" {
  description = "Root ID of the organization"
  value       = aws_organizations_organization.org.roots[0].id
}

output "ou_security_id" {
  description = "Security OU ID"
  value       = aws_organizations_organizational_unit.security.id
}

output "ou_infrastructure_id" {
  description = "Infrastructure OU ID"
  value       = aws_organizations_organizational_unit.infrastructure.id
}

output "ou_workloads_id" {
  description = "Workloads OU ID (parent of Production and Staging)"
  value       = aws_organizations_organizational_unit.workloads.id
}

output "ou_workloads_production_id" {
  description = "Production OU ID (child of Workloads)"
  value       = aws_organizations_organizational_unit.workloads_production.id
}

output "ou_workloads_staging_id" {
  description = "Staging OU ID (child of Workloads)"
  value       = aws_organizations_organizational_unit.workloads_staging.id
}

output "ou_sandbox_id" {
  description = "Sandbox OU ID"
  value       = aws_organizations_organizational_unit.sandbox.id
}
