# Generate SSH key pair
resource "tls_private_key" "albumstore" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "albumstore" {
  key_name   = "albumstore-key"
  public_key = tls_private_key.albumstore.public_key_openssh
}

# Save private key locally (never commit this file)
resource "local_file" "private_key" {
  content         = tls_private_key.albumstore.private_key_pem
  filename        = "${path.module}/albumstore-key.pem"
  file_permission = "0600"
}

# Latest Amazon Linux 2023 AMI
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_instance" "server" {
  ami                    = data.aws_ami.al2023.id
  instance_type          = "t3.medium"
  key_name               = aws_key_pair.albumstore.key_name
  vpc_security_group_ids = [aws_security_group.ec2.id]
  iam_instance_profile   = "LabInstanceProfile"

  tags = { Name = "albumstore-server" }
}

resource "aws_eip" "server" {
  instance = aws_instance.server.id
  domain   = "vpc"

  tags = { Name = "albumstore-eip" }
}
