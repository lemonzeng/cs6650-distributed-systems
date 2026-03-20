# HW8: MySQL + DynamoDB Shopping Cart Services
#
# Architecture:
#   Internet → ALB-MySQL  → ECS MySQL service  → RDS MySQL (private)
#   Internet → ALB-DynamoDB → ECS DynamoDB service → DynamoDB (managed)

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# ── Shared network (VPC, subnets, security groups) ─────────────────────────
module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
}

# ── CloudWatch log groups ───────────────────────────────────────────────────
module "logging_mysql" {
  source            = "./modules/logging"
  service_name      = "${var.service_name}-mysql"
  retention_in_days = var.log_retention_days
}

module "logging_dynamodb" {
  source            = "./modules/logging"
  service_name      = "${var.service_name}-dynamodb"
  retention_in_days = var.log_retention_days
}

# ── ECR images ──────────────────────────────────────────────────────────────
resource "aws_ecr_repository" "mysql" {
  name                 = var.mysql_ecr_repo
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}

resource "aws_ecr_repository" "dynamodb" {
  name                 = var.dynamodb_ecr_repo
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}

# ── RDS MySQL ───────────────────────────────────────────────────────────────
module "rds" {
  source          = "./modules/rds"
  service_name    = var.service_name
  db_name         = var.db_name
  db_username     = var.db_username
  db_password     = var.db_password
  subnet_ids      = module.network.subnet_ids
  rds_sg_id       = module.network.rds_security_group_id
}

# ── DynamoDB table ──────────────────────────────────────────────────────────
module "dynamodb" {
  source     = "./modules/dynamodb"
  table_name = var.dynamodb_table_name
}

# ── ALB for MySQL service ───────────────────────────────────────────────────
module "alb_mysql" {
  source                = "./modules/alb"
  service_name          = "${var.service_name}-mysql"
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
  container_port        = var.container_port
}

# ── ALB for DynamoDB service ────────────────────────────────────────────────
module "alb_dynamodb" {
  source                = "./modules/alb"
  service_name          = "${var.service_name}-dynamodb"
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
  container_port        = var.container_port
}

# ── ECS MySQL service ───────────────────────────────────────────────────────
module "ecs_mysql" {
  source             = "./modules/ecs"
  service_name       = "${var.service_name}-mysql"
  image              = "${aws_ecr_repository.mysql.repository_url}:latest"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.ecs_security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging_mysql.log_group_name
  region             = var.aws_region
  target_group_arn   = module.alb_mysql.target_group_arn
  alb_listener_arn   = module.alb_mysql.listener_arn
  env_vars = [
    {
      name  = "DB_DSN"
      value = "${var.db_username}:${var.db_password}@tcp(${module.rds.endpoint})/${var.db_name}?parseTime=true"
    }
  ]
}

# ── ECS DynamoDB service ────────────────────────────────────────────────────
module "ecs_dynamodb" {
  source             = "./modules/ecs"
  service_name       = "${var.service_name}-dynamodb"
  image              = "${aws_ecr_repository.dynamodb.repository_url}:latest"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.ecs_security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging_dynamodb.log_group_name
  region             = var.aws_region
  target_group_arn   = module.alb_dynamodb.target_group_arn
  alb_listener_arn   = module.alb_dynamodb.listener_arn
  env_vars = [
    {
      name  = "TABLE_NAME"
      value = var.dynamodb_table_name
    },
    {
      name  = "AWS_REGION"
      value = var.aws_region
    }
  ]
}
