# Part 3: Horizontal Scaling with ALB + Auto Scaling
# Reuses the same ECR image from Part 2 — no rebuild needed.
#
# Architecture:
#   Internet → ALB (port 80) → Target Group → ECS tasks (port 8080)
#   CloudWatch CPU > 50% → Auto Scaling adds tasks (min 2, max 4)

# Reference the existing ECR repo from Part 2 (no new image needed)
data "aws_ecr_repository" "app" {
  name = var.ecr_repository_name
}

# Reuse the existing IAM role
data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
}

module "alb" {
  source                = "./modules/alb"
  service_name          = var.service_name
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
  container_port        = var.container_port
}

module "logging" {
  source            = "./modules/logging"
  service_name      = var.service_name
  retention_in_days = var.log_retention_days
}

module "ecs" {
  source             = "./modules/ecs"
  service_name       = var.service_name
  image              = "${data.aws_ecr_repository.app.repository_url}:latest"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.ecs_security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging.log_group_name
  region             = var.aws_region
  target_group_arn   = module.alb.target_group_arn
  alb_listener_arn   = module.alb.listener_arn # implicit dep: ECS waits for ALB
  min_capacity       = var.min_capacity
  max_capacity       = var.max_capacity
  cpu_target         = var.cpu_target
}
