output "target_group_arn" {
  value = aws_lb_target_group.this.arn
}

output "listener_arn" {
  value = aws_lb_listener.this.arn
}

output "dns_name" {
  value = aws_lb.this.dns_name
}
