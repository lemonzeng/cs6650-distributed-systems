output "server_ip" {
  description = "Public IP of the EC2 instance"
  value       = aws_eip.server.public_ip
}

output "rds_endpoint" {
  description = "RDS MySQL endpoint (host:port)"
  value       = aws_db_instance.mysql.endpoint
}

output "s3_bucket" {
  description = "S3 bucket name"
  value       = aws_s3_bucket.photos.bucket
}

output "base_url" {
  description = "Your service base URL — use this for ChaosArena submission"
  value       = "http://${aws_eip.server.public_ip}:8080"
}

output "ssh_command" {
  description = "SSH into EC2"
  value       = "ssh -i infra/albumstore-key.pem ec2-user@${aws_eip.server.public_ip}"
}

output "db_dsn" {
  description = "Go DB DSN — paste into .env on EC2"
  value       = "${var.db_username}:${var.db_password}@tcp(${aws_db_instance.mysql.endpoint})/albumstore?parseTime=true"
  sensitive   = true
}
