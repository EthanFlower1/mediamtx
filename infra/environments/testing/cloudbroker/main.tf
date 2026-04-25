terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# --- Variables ---

variable "region" {
  default = "us-east-2"
}

variable "domain" {
  description = "Domain for the broker (e.g., connect.raikada.com)"
  default     = "connect.raikada.com"
}

variable "broker_token" {
  description = "Bearer token for Directory authentication"
  sensitive   = true
}

variable "ssh_key_name" {
  description = "Name of an existing EC2 key pair for SSH access (optional)"
  default     = ""
}

# --- Networking ---

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
  filter {
    name   = "default-for-az"
    values = ["true"]
  }
}

resource "aws_security_group" "broker" {
  name_prefix = "raikada-broker-"
  description = "Cloud broker - HTTPS, WSS, SSH"
  vpc_id      = data.aws_vpc.default.id

  # SSH
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "SSH"
  }

  # HTTP (for Let's Encrypt ACME challenge)
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "HTTP - ACME challenge"
  }

  # HTTPS + WSS
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "HTTPS / WSS"
  }

  # FRP control port
  ingress {
    from_port   = 7000
    to_port     = 7000
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "FRP tunnel control"
  }

  # TURN server (TCP)
  ingress {
    from_port   = 3478
    to_port     = 3478
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "TURN TCP"
  }

  # TURN server (UDP)
  ingress {
    from_port   = 3478
    to_port     = 3478
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "TURN UDP"
  }

  # TURN relay port range (UDP)
  ingress {
    from_port   = 49152
    to_port     = 49252
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "TURN relay ports"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "raikada-broker"
  }
}

# --- EC2 Instance ---

data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-*-arm64"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_instance" "broker" {
  ami                    = data.aws_ami.amazon_linux.id
  instance_type          = "t4g.micro" # ARM, ~$6/mo
  subnet_id              = data.aws_subnets.default.ids[0]
  vpc_security_group_ids = [aws_security_group.broker.id]
  key_name               = var.ssh_key_name != "" ? var.ssh_key_name : null

  associate_public_ip_address = true

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
  }

  user_data = base64encode(templatefile("${path.module}/user_data.sh", {
    domain       = var.domain
    broker_token = var.broker_token
  }))

  tags = {
    Name = "raikada-broker"
  }
}

# --- Elastic IP (stable across instance replacements) ---

resource "aws_eip" "broker" {
  domain = "vpc"
  tags = {
    Name = "raikada-broker"
  }
}

resource "aws_eip_association" "broker" {
  instance_id   = aws_instance.broker.id
  allocation_id = aws_eip.broker.id
}

# --- Outputs ---

output "public_ip" {
  value       = aws_eip.broker.public_ip
  description = "Stable IP — set this as the A record for connect.raikada.com in Cloudflare"
}

output "broker_url" {
  value       = "wss://${var.domain}/ws/directory"
  description = "Set this as MTX_CLOUDCONNECTURL on your Directory"
}

output "ssh_command" {
  value       = var.ssh_key_name != "" ? "ssh -i ~/.ssh/raikada-broker.pem ec2-user@${aws_eip.broker.public_ip}" : "No SSH key configured"
  description = "SSH into the broker"
}

output "health_check" {
  value       = "curl https://${var.domain}/healthz"
  description = "Verify the broker is running"
}
