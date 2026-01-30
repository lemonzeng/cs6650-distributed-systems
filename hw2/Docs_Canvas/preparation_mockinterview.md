## 1. papaer review - share my thought about the paper
## 2. Part2:
### 2.1 explain ` main.tf` code
### 2.2 show to process:
Terminal Commands:
+ AWS
    + aws configure
    + aws configure set aws_session_token + token
    + aws sts get-caller-identity
+ Create Instance
    + terraform init
    + terraform apply -auto-approve
+ Connect via SSH
    + ssh -i "YumengZeng-cs6650-hw1b.pem" ec2-user@ec2-34-215-204-171.us-west-2.compute.amazonaws.com

## 3. Part3: Docker 
### 3.1 Build and run image in local
+ explain Dockerfile
+ create a new terminal in the current folder (hw2) & run commands
    + docker build --tag godocker-test1 .
    + docker image ls
    + docker run -d -p 8080:8080 --name my-app godocker-test1
    + docker ps
### 3.2 Build and run image in the Cloud
+ create a new EC2 instance
+ then, install Git and Docker
    + sudo yum update -y
    + sudo yum install git -y
    + sudo yum install docker -y
    + sudo service docker start
    + sudo usermod -a -G docker ec2-user
    + then, exit (disconnect ssh) and reconnect
    + git clone https://github.com/lemonzeng/cs6650-distributed-systems.git
    + token: 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
    + cd cs6650-distributed-systems/hw2
+ [!Important] Build docker in the instance
+ be careful! It would OOM.![alt text](image.png)
> Why Swap? (Concise Version)
    > 1. The Problem
    >- Hardware: t2.micro has only 1GB RAM.
    >- The Build: go build is memory-heavy. It exceeds 1GB when running inside Docker.
    >- Crash: Linux triggers the OOM Killer (Out of Memory) to save the system, which kills your build process and disconnects your SSH session.
    > 2. The Principle (Disk as RAM)
    >- Virtual Memory: Swap uses your Hard Drive as temporary RAM.
    >- Swapping: When RAM is full, the system moves inactive data to the disk to free up physical memory for the compiler.
    >- Trade-off: It prevents crashes (Safety) but is much slower than real RAM (Performance).


+ Setup & Deployment (7 Steps)

    Step 1: Create 2GB Swap File

    `sudo dd if=/dev/zero of=/swapfile bs=128M count=16`

    
    Step 2: Secure Permissions

    `sudo chmod 600 /swapfile`


    Step 3: Format as Swap

    `sudo mkswap /swapfile`


    Step 4: Enable Swap

    `sudo swapon /swapfile`


    Step 5: Verify (Look for 2.0G in Swap row)

    `free -h`


    Step 6: Build Docker Image

    `docker build -t godocker-test1 .`


    Step 7: Run & Test

    `docker run -d -p 8080:8080 --name godocker-test1 godocker-test1`
    `curl http://<Public-IP>:8080/albums`



### 3.3 create a new instance and repeat building image
## Part 4: test inconsistency
+ activate vitual environment: source venv/bin/activate
+ python3 test_consistency.py

## 4. Clean Up
`terraform destroy -auto-approve`