# KAI-231: ALB sub-module stub.
# KAI-230 (region routing) will add listener rules that route by
# X-Kaivue-Region header and host-based routing for
# https://us-east-2.api.yourbrand.com/v1/...
# ACM cert validation is via Route53 DNS (managed in modules/global/route53).

resource "aws_security_group" "alb" {
  name        = "kaivue-${var.environment}-${var.region}-alb"
  description = "ALB public ingress"
  vpc_id      = var.vpc_id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = var.tags
}

resource "aws_lb" "main" {
  name               = "kaivue-${var.environment}-${var.region}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.subnet_ids

  enable_deletion_protection = var.environment == "production" ? true : false

  tags = var.tags
}

resource "aws_lb_target_group" "api" {
  name     = "kaivue-${var.environment}-${var.region}-api"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = var.vpc_id

  health_check {
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
  }

  tags = var.tags
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}
