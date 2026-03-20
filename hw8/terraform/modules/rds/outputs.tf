output "endpoint" {
  description = "RDS endpoint in host:port format"
  value       = aws_db_instance.this.endpoint
}
