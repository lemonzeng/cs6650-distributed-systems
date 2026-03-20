
# RDS MySQL 8.0 on db.t3.micro (free tier)
# Placed in the default VPC subnets; accessible only from ECS security group.

resource "aws_db_subnet_group" "this" {
  name       = "${lower(var.service_name)}-db-subnet-group"
  subnet_ids = var.subnet_ids

  tags = {
    Name = "${var.service_name}-db-subnet-group"
  }
}

resource "aws_db_instance" "this" {
  identifier        = "${lower(var.service_name)}-mysql"
  engine            = "mysql"
  engine_version    = "8.0"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.rds_sg_id]

  publicly_accessible     = false
  skip_final_snapshot     = true   # for assignments — no snapshot on destroy
  deletion_protection     = false
  backup_retention_period = 0      # disable automated backups (saves cost)

  tags = {
    Name = "${var.service_name}-mysql"
  }
}
