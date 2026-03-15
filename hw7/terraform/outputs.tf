output "alb_dns_name" {
  description = "DNS name of the ALB — use this as the Locust target host"
  value       = "http://${aws_lb.main.dns_name}"
}

output "sns_topic_arn" {
  description = "ARN of the SNS topic (needed for Part III Lambda trigger)"
  value       = aws_sns_topic.orders.arn
}

output "sqs_queue_url" {
  description = "URL of the SQS queue"
  value       = aws_sqs_queue.orders.url
}

output "sqs_queue_arn" {
  description = "ARN of the SQS queue"
  value       = aws_sqs_queue.orders.arn
}

output "ecr_receiver_url" {
  description = "ECR repository URL for order-receiver — push your image here"
  value       = aws_ecr_repository.receiver.repository_url
}

output "ecr_processor_url" {
  description = "ECR repository URL for order-processor — push your image here"
  value       = aws_ecr_repository.processor.repository_url
}

output "ecs_cluster_name" {
  description = "ECS cluster name"
  value       = aws_ecs_cluster.main.name
}

output "cloudwatch_receiver_log_group" {
  description = "CloudWatch log group for order-receiver"
  value       = aws_cloudwatch_log_group.receiver.name
}

output "cloudwatch_processor_log_group" {
  description = "CloudWatch log group for order-processor"
  value       = aws_cloudwatch_log_group.processor.name
}
