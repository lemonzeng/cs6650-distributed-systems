# CS6650 HW5: Scalable E-commerce Product API

This repository contains a concurrent-safe RESTful API built with Go (Gin) and deployed on AWS Fargate using Terraform.

## Project Structure and Location

The project is organized as follows:

* Server Code: `/src/main.go`
* Dependencies: `/src/go.mod`, `/src/go.sum`
* Dockerfile: `/src/Dockerfile`
* Infrastructure (Terraform): `/terraform/`
* Load Tests: `/tests/locustfile.py`
* Documentation: `README.md`

## Deployment Instructions

### 1. Cloud Deployment (AWS Fargate)

To deploy the infrastructure on a different machine:

1. **Configure AWS Credentials**: Retrieve temporary credentials from AWS Learner's Lab and set them:

```bash
aws configure
# Enter Access Key and Secret Key
aws configure set aws_session_token <YOUR_SESSION_TOKEN>
```

2. **Initialize and Apply Infrastructure**:

```bash
cd terraform
terraform init
terraform apply -auto-approve
```

3. **Retrieve Fargate Public IP**:

```bash
aws ec2 describe-network-interfaces --network-interface-ids $(aws ecs describe-tasks --cluster $(terraform output -raw ecs_cluster_name) --tasks $(aws ecs list-tasks --cluster $(terraform output -raw ecs_cluster_name) --query 'taskArns[0]' --output text) --query "tasks[0].attachments[0].details[?name=='networkInterfaceId'].value" --output text) --query 'NetworkInterfaces[0].Association.PublicIp' --output text
```

### 2. Local Deployment (Docker)

1. **Build Image**:

```bash
cd src
docker build -t product-api .
```

2. **Run Container**:

```bash
docker run -p 8080:8080 product-api
```

## API Endpoints and Response Code Examples

Replace `<IP>` with your public IP or localhost.

### 1. GET /products/:productId

Retrieve product details by ID.

#### 200 OK

Description: Successfully retrieved an existing product (ensure you POST the ID first).

```bash
curl -i http://<IP>:8080/products/1
```

Response Body:

```json
{"product_id":1,"sku":"MAC-PRO","manufacturer":"Apple","category_id":1,"weight":1500,"some_other_id":100}
```

#### 400 Bad Request

Description: Invalid product ID (e.g., non-integer or ID < 1).

```bash
curl -i http://<IP>:8080/products/abc
```

Response Body:

```json
{"error":"INVALID_ID","message":"The provided input data is invalid","details":"Product ID must be a positive integer"}
```

#### 404 Not Found

Description: Product ID does not exist in the memory store.

```bash
curl -i http://<IP>:8080/products/9999
```

Response Body:

```json
{"error":"NOT_FOUND","message":"Product not found","details":"The requested product ID does not exist in memory"}
```

### 2. POST /products/:productId/details

Add or update product information.

#### 204 No Content

Description: Successfully updated the product data.

```bash
curl -i -X POST http://<IP>:8080/products/1/details \
-H "Content-Type: application/json" \
-d '{"sku": "MAC-PRO", "manufacturer": "Apple", "category_id": 1}'
```

Response: `HTTP/1.1 204 No Content` (No body)

#### 400 Bad Request (Invalid ID)

Description: Product ID in URL is invalid.

```bash
curl -i -X POST http://<IP>:8080/products/0/details \
-H "Content-Type: application/json" \
-d '{"sku": "TEST", "manufacturer": "TEST"}'
```

Response Body:

```json
{"error":"INVALID_INPUT","message":"The provided input data is invalid","details":"Product ID must be a positive integer"}
```

#### 400 Bad Request (Invalid JSON/Missing Fields)

Description: Missing required fields like "sku" or "manufacturer" in JSON body.

```bash
curl -i -X POST http://<IP>:8080/products/1/details \
-H "Content-Type: application/json" \
-d '{"category_id": 1}'
```

Response Body:

```json
{"error":"INVALID_INPUT","message":"Invalid data provided","details":"Key: 'Product.SKU' Error:Field validation for 'SKU' failed on the 'required' tag..."}
```

## Performance Testing

Load tests were conducted using Locust located in `/tests/locustfile.py`.

* Execution: `locust -f tests/locustfile.py --host http://<IP>:8080`
* Full performance bottleneck analysis is documented in `performance_report.md`.

## Security and Git Configuration

The `.gitignore` file is configured to exclude:

* Terraform state files (`*.tfstate`, `*.tfstate.backup`)
* Sensitive variable files (`*.tfvars`)
* Compiled binaries and large Docker cache files
* Local environment variables
