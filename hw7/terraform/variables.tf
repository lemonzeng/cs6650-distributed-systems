variable "aws_region" {
  description = "AWS region to deploy into"
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Prefix used for all resource names"
  type        = string
  default     = "hw7"
}

variable "receiver_image" {
  description = "Full ECR image URI for order-receiver (e.g. 123456789.dkr.ecr.us-east-1.amazonaws.com/hw7-receiver:latest)"
  type        = string
}

variable "processor_image" {
  description = "Full ECR image URI for order-processor (e.g. 123456789.dkr.ecr.us-east-1.amazonaws.com/hw7-processor:latest)"
  type        = string
}

variable "worker_count" {
  description = "Number of concurrent worker goroutines in the order-processor task. Adjust for Phase 5 scaling experiments."
  type        = string
  default     = "1"
}
