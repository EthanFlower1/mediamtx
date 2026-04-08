variable "aws_account_id" {
  description = "Production AWS account ID — supplied via CI secret"
  type        = string
}

variable "root_domain" {
  description = "Root DNS domain for production (e.g. yourbrand.com)"
  type        = string
}
