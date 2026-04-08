# KAI-231: VPC sub-module stub (KAI-215 will flesh out NAT gateways,
# VPC endpoints for S3/ECR/CloudWatch, and flow logs to CloudWatch).
# Subnet CIDR allocation: /20 per AZ for private, /24 per AZ for public.

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = merge(var.tags, {
    Name = "kaivue-${var.environment}-${var.region}"
  })
}

resource "aws_subnet" "private" {
  count             = length(var.availability_zones)
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 4, count.index)
  availability_zone = var.availability_zones[count.index]
  tags = merge(var.tags, {
    Name = "kaivue-${var.environment}-${var.region}-private-${count.index}"
    Tier = "private"
  })
}

resource "aws_subnet" "public" {
  count                   = length(var.availability_zones)
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, 128 + count.index)
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = true
  tags = merge(var.tags, {
    Name = "kaivue-${var.environment}-${var.region}-public-${count.index}"
    Tier = "public"
  })
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags = merge(var.tags, {
    Name = "kaivue-${var.environment}-${var.region}-igw"
  })
}
