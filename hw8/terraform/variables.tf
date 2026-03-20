variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "mysql_ecr_repo" {
  type        = string
  description = "ECR repository name for the MySQL service image"
  default     = "ecr-hw8-mysql"
}

variable "dynamodb_ecr_repo" {
  type        = string
  description = "ECR repository name for the DynamoDB service image"
  default     = "ecr-hw8-dynamodb"
}

variable "service_name" {
  type    = string
  default = "CS6650HW8"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "log_retention_days" {
  type    = number
  default = 7
}

variable "db_name" {
  type    = string
  default = "cartdb"
}

variable "db_username" {
  type    = string
  default = "admin"
}

variable "db_password" {
  type        = string
  sensitive   = true
  description = "RDS MySQL master password — set via TF_VAR_db_password env var"
}

variable "dynamodb_table_name" {
  type    = string
  default = "shopping-carts"
}
