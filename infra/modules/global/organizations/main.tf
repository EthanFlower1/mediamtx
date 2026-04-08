# KAI-231: AWS Organizations stub.
# KAI-214 (Bootstrap AWS account structure) owns the live implementation.
# Service control policies (SCPs) for multi-tenant isolation enforcement
# will be added here — production SCPs must deny cross-tenant IAM assumptions.
#
# NOTE: aws_organizations_organization is a singleton; only create in the
# management/root account. Child account references are data sources.
