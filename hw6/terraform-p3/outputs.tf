# Use this DNS name as --host in Locust for Part 3 testing
output "alb_dns_name" {
  description = "ALB DNS name - use as Locust host"
  value       = "http://${module.alb.dns_name}"
}

output "ecs_cluster_name" {
  value = module.ecs.cluster_name
}

output "ecs_service_name" {
  value = module.ecs.service_name
}
