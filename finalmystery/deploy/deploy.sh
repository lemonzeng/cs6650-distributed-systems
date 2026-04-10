#!/bin/bash
set -e
EC2_IP="35.160.126.130"
KEY="infra/albumstore-key.pem"
SSH="ssh -i $KEY -o StrictHostKeyChecking=no ec2-user@$EC2_IP"
SCP="scp -i $KEY -o StrictHostKeyChecking=no"

echo "Building linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o albumstore ./cmd/server

echo "Provisioning EC2 directory..."
$SSH "sudo mkdir -p /opt/albumstore && sudo chown ec2-user:ec2-user /opt/albumstore"

echo "Stopping service before overwrite..."
$SSH "sudo systemctl stop albumstore"

echo "Copying binary and env..."
$SCP albumstore ec2-user@$EC2_IP:/opt/albumstore/albumstore
$SCP .env ec2-user@$EC2_IP:/opt/albumstore/.env

echo "Installing systemd service..."
$SCP deploy/albumstore.service ec2-user@$EC2_IP:/tmp/albumstore.service
$SSH "sudo mv /tmp/albumstore.service /etc/systemd/system/albumstore.service && sudo systemctl daemon-reload && sudo systemctl enable albumstore"

echo "Restarting service..."
$SSH "sudo systemctl restart albumstore"

echo "Waiting 3s for startup..."
sleep 3
curl -sf http://$EC2_IP:8080/health && echo " Health check passed!" || echo " Health check failed — check logs"

echo "Done."
