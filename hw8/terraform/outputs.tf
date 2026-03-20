output "mysql_alb_url" {
  description = "URL for the MySQL shopping cart service"
  value       = "http://${module.alb_mysql.dns_name}"
}

output "dynamodb_alb_url" {
  description = "URL for the DynamoDB shopping cart service"
  value       = "http://${module.alb_dynamodb.dns_name}"
}

output "rds_endpoint" {
  description = "RDS MySQL endpoint (host:port)"
  value       = module.rds.endpoint
}

output "mysql_ecr_url" {
  description = "ECR URL to push MySQL service image"
  value       = aws_ecr_repository.mysql.repository_url
}

output "dynamodb_ecr_url" {
  description = "ECR URL to push DynamoDB service image"
  value       = aws_ecr_repository.dynamodb.repository_url
}
