# Hummingbird Codebase Walkthrough

## Architecture Overview

**Hummingbird** is a Node.js/Express media processing service backed by AWS. It has two runtime components:
- **API server** (`server.js`) — handles HTTP requests
- **Background worker** (`worker/processor.js`) — polls SQS and processes events asynchronously

**AWS services used:** S3 (file storage), DynamoDB (metadata), SNS (event pub), SQS (event sub)

---

## Upload → Download Lifecycle

### 1. `POST /v1/media/upload?width=500`

**Middlewares** (run in order before the controller):

| File | Function | What it does |
|------|----------|--------------|
| `middlewares/validateWidth.js` | `middleware` | Validates `width` query param is an integer within allowed bounds |
| `middlewares/setMediaWidth.js` | `middleware` | Parses width and attaches it to `req.hummingbirdOptions.width` |

**Controller → Action:**

| File | Function | What it does |
|------|----------|--------------|
| `controllers/media.js:27` | `uploadController` | Orchestrates the upload flow |
| `actions/uploadMedia.js:25` | `uploadMedia` | Uses `formidable` to parse the multipart request; streams the file directly to S3 via a `Transform` stream (no temp file on disk); generates a UUID as `mediaId` |
| `clients/s3.js:22` | `uploadMediaToStorage` | Writes the stream to S3 at `uploads/<mediaId>/<filename>` |
| `clients/dynamodb.js:23` | `createMedia` | Creates a DynamoDB record `MEDIA#<mediaId>` with status = `PENDING` |
| `clients/sns.js:56` | `publishResizeMediaEvent` | Publishes a `media.v1.resize` event to SNS, which fans out to SQS |

Response: `202 Accepted` with `{ mediaId }`.

---

### 2. Background Worker Processes the Event

| File | Function | What it does |
|------|----------|--------------|
| `worker/processor.js:95` | `pollForever` | Long-polls SQS in an infinite loop (20s wait, up to 10 messages) |
| `worker/processor.js:75` | `handleMessage` | Parses the SNS envelope from SQS body, routes by `type` |
| `worker/processor.js:30` | `handleResize` | Sets status to `PROCESSING`, copies `uploads/` to `resized/` in S3 (no actual pixel resize, just a copy), then sets status to `COMPLETE` |

---

### 3. `GET /v1/media/<id>/status`

| File | Function | What it does |
|------|----------|--------------|
| `controllers/media.js:80` | `statusController` | Reads DynamoDB record, returns `{ status }` — one of `PENDING`, `PROCESSING`, `COMPLETE` |

---

### 4. `GET /v1/media/<id>/download`

| File | Function | What it does |
|------|----------|--------------|
| `controllers/media.js:98` | `downloadController` | Reads DynamoDB to check status. If **not** `PROCESSING`, returns `202` with `Retry-After: 60`. If status is `PROCESSING`, calls `getProcessedMediaUrl` and redirects `302` to the presigned S3 URL |
| `clients/s3.js:54` | `getProcessedMediaUrl` | Generates a 1-hour presigned S3 `GetObject` URL for `uploads/<mediaId>/<filename>` |

> **Note:** There is a bug in `downloadController` — it checks `status !== PROCESSING` to gate the download, but the terminal "ready" state is actually `COMPLETE`. This means the download is only served during the brief window when status is `PROCESSING`, not after it transitions to `COMPLETE`.

---

## Data Flow Summary

```
POST /upload
  -> validate/parse width (middlewares)
  -> stream file to S3: uploads/<id>/<name>
  -> write DynamoDB: status=PENDING
  -> publish SNS: media.v1.resize
        | (async, via SQS)
        v
Worker polls SQS
  -> set DynamoDB: status=PROCESSING
  -> S3 copy: uploads/ -> resized/
  -> set DynamoDB: status=COMPLETE

GET /status   -> DynamoDB read -> { status }
GET /download -> DynamoDB check -> 302 redirect to S3 presigned URL
```

---

## Other Endpoints

| Route | Controller | What it does |
|-------|------------|--------------|
| `GET /v1/media/:id` | `getController` (`controllers/media.js:132`) | Returns full media metadata from DynamoDB |
| `PUT /v1/media/:id/resize` | `resizeController` (`controllers/media.js:149`) | Synchronously copies the file and sets status to `COMPLETE`, then publishes a resize event |
| `DELETE /v1/media/:id` | `deleteController` (`controllers/media.js:176`) | Publishes a `media.v1.delete` event; worker deletes S3 files and DynamoDB record |

---

## Key Files Reference

```
server.js                        Express app entry point
routes/media.js                  Route definitions
controllers/media.js             Request handlers for all media routes
actions/uploadMedia.js           Streaming multipart upload logic
worker/processor.js              SQS polling + event handler (resize, delete)
clients/s3.js                    S3 operations (upload, download, copy, delete)
clients/dynamodb.js              DynamoDB operations (create, get, update, delete)
clients/sns.js                   SNS publish helpers
middlewares/validateWidth.js     Width query param validation
middlewares/setMediaWidth.js     Width extraction and normalization
core/constants.js                Shared constants (status values, event types, limits)
```

---

## Terraform Outputs (terraform output)

```
alb_dns_name           = "hummingbird-production-alb-217067988.us-west-2.elb.amazonaws.com"
dynamodb_table_name    = "hummingbird-app-table"
ecr_repository_url     = "260256919823.dkr.ecr.us-west-2.amazonaws.com/hummingbird-production-api"
ecs_cluster_name       = "humming-bird-cluster"
media_events_queue_url = "https://sqs.us-west-2.amazonaws.com/260256919823/hummingbird-production-media-events"
s3_bucket_name         = "hummingbird-production-260256919823"
sns_topic_arn          = "arn:aws:sns:us-west-2:260256919823:hummingbird-production-media-management-topic"
```
