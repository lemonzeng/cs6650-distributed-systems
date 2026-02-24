variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "ecr_repository_name" {
  type    = string
  default = "ecr-hw6-search" # reuse the same image from Part 2
}

variable "service_name" {
  type    = string
  default = "CS6650HW6P3"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "log_retention_days" {
  type    = number
  default = 7
}

variable "min_capacity" {
  type        = number
  default     = 2
  description = "Minimum number of ECS tasks (start with 2 so ALB has capacity)"
}

variable "max_capacity" {
  type        = number
  default     = 4
  description = "Maximum number of ECS tasks auto scaling can launch"
}

variable "cpu_target" {
  type        = number
  default     = 50
  description = "Target CPU utilization % that triggers auto scaling"
}
