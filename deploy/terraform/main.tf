terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket = "kashport-terraform-state"
    key    = "kp-gruuk/terraform.tfstate"
    region = "us-east-1"
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  default = "us-east-1"
}

variable "domain" {
  default = "gk.kspt.dev"
}

variable "okta_issuer" {
  type = string
}

variable "okta_client_id" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "hosted_zone_id" {
  type        = string
  description = "Route53 hosted zone ID for kspt.dev"
}

# ACM Certificate for *.gk.kspt.dev
resource "aws_acm_certificate" "gruuk" {
  domain_name               = var.domain
  subject_alternative_names = ["*.${var.domain}"]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.gruuk.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id = var.hosted_zone_id
  name    = each.value.name
  type    = each.value.type
  records = [each.value.record]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "gruuk" {
  certificate_arn         = aws_acm_certificate.gruuk.arn
  validation_record_fqdns = [for r in aws_route53_record.cert_validation : r.fqdn]
}

# ALB
resource "aws_security_group" "alb" {
  name_prefix = "gruuk-alb-"
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
}

resource "aws_lb" "gruuk" {
  name               = "gruuk"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.subnet_ids

  idle_timeout = 3600 # 1 hour for WebSocket connections
}

resource "aws_lb_target_group" "gruuk" {
  name        = "gruuk"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/_health"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    timeout             = 5
    interval            = 30
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.gruuk.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.gruuk.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.gruuk.arn
  }
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.gruuk.arn
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

# Route53 records
resource "aws_route53_record" "base" {
  zone_id = var.hosted_zone_id
  name    = var.domain
  type    = "A"

  alias {
    name                   = aws_lb.gruuk.dns_name
    zone_id                = aws_lb.gruuk.zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "wildcard" {
  zone_id = var.hosted_zone_id
  name    = "*.${var.domain}"
  type    = "A"

  alias {
    name                   = aws_lb.gruuk.dns_name
    zone_id                = aws_lb.gruuk.zone_id
    evaluate_target_health = true
  }
}

# ECS
resource "aws_ecs_cluster" "gruuk" {
  name = "gruuk"
}

resource "aws_security_group" "ecs" {
  name_prefix = "gruuk-ecs-"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_iam_role" "ecs_task_execution" {
  name = "gruuk-ecs-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_task_execution" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_cloudwatch_log_group" "gruuk" {
  name              = "/ecs/gruuk"
  retention_in_days = 30
}

resource "aws_ecs_task_definition" "gruuk" {
  family                   = "gruuk"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 512
  memory                   = 1024
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn

  container_definitions = jsonencode([{
    name  = "gruuk-server"
    image = "ghcr.io/kashportsa/kp-gruuk:latest"
    portMappings = [{ containerPort = 8080, protocol = "tcp" }]
    environment = [
      { name = "DOMAIN", value = var.domain },
      { name = "LISTEN_ADDR", value = ":8080" },
      { name = "OKTA_ISSUER", value = var.okta_issuer },
      { name = "OKTA_CLIENT_ID", value = var.okta_client_id },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.gruuk.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "gruuk"
      }
    }
  }])
}

resource "aws_ecs_service" "gruuk" {
  name            = "gruuk"
  cluster         = aws_ecs_cluster.gruuk.id
  task_definition = aws_ecs_task_definition.gruuk.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [aws_security_group.ecs.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.gruuk.arn
    container_name   = "gruuk-server"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.https]
}

output "alb_dns" {
  value = aws_lb.gruuk.dns_name
}

output "public_url" {
  value = "https://${var.domain}"
}
