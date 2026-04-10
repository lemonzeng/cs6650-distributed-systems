variable "db_username" {
  description = "RDS MySQL username"
  type        = string
  default     = "albumuser"
}

variable "db_password" {
  description = "RDS MySQL password"
  type        = string
  default     = "AlbumStore2024!"
  sensitive   = true
}
