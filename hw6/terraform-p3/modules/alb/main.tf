# Application Load Balancer
# Distributes requests across all healthy ECS tasks.
# Health check path /health ensures only ready tasks receive traffic.

resource "aws_lb" "this" {
  name               = "${var.service_name}-alb"
  internal           = false # internet-facing
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.subnet_ids
}

# Target Group: where ALB sends requests
# target_type = "ip" is required for Fargate (tasks don't have fixed EC2 instances)
resource "aws_lb_target_group" "this" {
  name        = "${var.service_name}-tg"
  port        = var.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip" # required for Fargate

  health_check {
    path                = "/health"
    interval            = 30 # check every 30s
    healthy_threshold   = 2  # 2 consecutive successes = healthy
    unhealthy_threshold = 3
    timeout             = 5
  }
}

# Listener: ALB listens on port 80, forwards to target group
resource "aws_lb_listener" "this" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }
}
