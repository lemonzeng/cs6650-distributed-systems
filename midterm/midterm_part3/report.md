# Midterm Submission Report

---

## Ticket #1 — "Server started on the wrong port" (Easy)

### Bug Found
- **File:** `server.js`, **line 35**
- **Bug:** `const port = process.env.APP_PORT;` reads the port directly from the environment variable with no fallback. If `APP_PORT` is not set in `.env`, `port` is `undefined`. When Node.js calls `app.listen(undefined)`, it doesn't crash — it silently picks a random ephemeral port, and the log outputs `listening on port undefined`.

### Code Diff
```diff
- const port = process.env.APP_PORT;
+ const port = process.env.APP_PORT || 9000;
```

---

## Ticket #2 — "Width is missing from metadata" (Easy)

### Bug Found
- **File:** `clients/dynamodb.js`, **lines 78–84** (inside `getMedia`)
- **Bug:** The `createMedia` function saves `width` to DynamoDB (line 34: `width: { N: String(width) }`), but `getMedia` never reads it back. The return object includes `size`, `name`, `mimetype`, and `status` — but `width` is completely missing.

### Code Diff
```diff
      return {
        mediaId,
        size: Number(Item.size.N),
        name: Item.name.S,
        mimetype: Item.mimetype.S,
        status: Item.status.S,
+       width: Number(Item.width.N),
      };
```

### Verification
```
$ curl -X POST "http://<alb-dns>/v1/media/upload?width=800" -F "file=@sample.jpg"
{"mediaId":"3f424d40-fe18-458a-b2aa-530685918ca1"}

$ curl http://<alb-dns>/v1/media/3f424d40-fe18-458a-b2aa-530685918ca1
{"mediaId":"3f424d40-fe18-458a-b2aa-530685918ca1","size":10240,"name":"sample.jpg","mimetype":"image/jpeg","status":"PENDING","width":800}
```
`width: 800` now appears in the response.

---

## Ticket #3 — "Your redirect URL is broken" (Intermediate)

### Bug Found
- **File:** `controllers/media.js`, **line 111**
- **Bug:** `res.set('Location', \`${req.hostname}/v1/media/${mediaId}/status\`)` — the `Location` header is missing the `http://` protocol prefix. `req.hostname` in Express returns only the hostname (e.g., `hummingbird-alb-xxx.elb.amazonaws.com`), not a full URL. Without the scheme, HTTP clients cannot parse or follow the redirect.

### Investigation Notes
- `req.hostname` returns only the domain name, stripping the port if present.
- `req.get('host')` returns the `Host` header value, which includes the port if non-standard.
- Neither includes a protocol scheme (`http://` or `https://`).
- A valid `Location` header per RFC 7231 must be a full absolute URI: `http://hostname/path`.

### Code Diff
```diff
-      res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);
+      res.set('Location', `http://${req.hostname}/v1/media/${mediaId}/status`);
```

### Verification
Before fix:
```
Location: hummingbird-production-alb-1405483471.us-west-2.elb.amazonaws.com/v1/media/.../status
```
After fix:
```
Location: http://hummingbird-production-alb-1405483471.us-west-2.elb.amazonaws.com/v1/media/.../status
```

---

## Ticket #4 — "Download never redirects even when COMPLETE" (Intermediate)

### Bug Found
- **File:** `controllers/media.js`, **line 108**
- **Bug:** `if (media.status !== MEDIA_STATUS.PROCESSING)` — the condition is inverted. This checks if the status is NOT `PROCESSING`, and if so, returns a 202. But `COMPLETE` is not `PROCESSING`, so COMPLETE always triggers the 202 "still processing" response. Meanwhile, `PROCESSING` falls through to the 302 redirect — the exact opposite of intended behavior.

### Logic Table (Before Fix)
| `media.status` | `!== PROCESSING` | Result          |
|----------------|-------------------|-----------------|
| `PENDING`      | `true`            | 202 (correct)   |
| `PROCESSING`   | `false`           | 302 (WRONG)     |
| `COMPLETE`     | `true`            | 202 (WRONG)     |
| `ERROR`        | `true`            | 202 (correct)   |

### Code Diff
```diff
-    if (media.status !== MEDIA_STATUS.PROCESSING) {
+    if (media.status !== MEDIA_STATUS.COMPLETE) {
```

### Verification
After fix, with status COMPLETE:
```
$ curl -i http://<alb-dns>/v1/media/<mediaId>/download
HTTP/1.1 302 Found
Location: https://hummingbird-production-260256919823.s3.us-west-2.amazonaws.com/uploads/<mediaId>/sample.jpg?...
```

---

## Bonus — "Status never changes. No errors. Nothing." (Advanced)

### Bug Found
- **File:** `clients/dynamodb.js`, **line 155** (SK key) and **line 167** (logger)
- **Bug:** `setMediaStatus` uses `SK: { S: 'metadata' }` (lowercase), while all other functions (`createMedia`, `getMedia`, `setMediaStatusConditionally`, `deleteMedia`) use `SK: { S: 'METADATA' }` (uppercase). DynamoDB keys are case-sensitive, so `setMediaStatus` was writing to a **completely different item** — a phantom record that no other function ever reads.

### Why There Were Zero Errors
DynamoDB's `UpdateItem` with no `ConditionExpression` silently **creates a new item** if the specified key doesn't exist. So the update always "succeeded" — it just wrote to a different row. No error, no crash, no indication anything was wrong.

### DynamoDB Key Comparison
| Function                      | SK Value      | Line |
|-------------------------------|---------------|------|
| `createMedia`                 | `'METADATA'`  | 29   |
| `getMedia`                    | `'METADATA'`  | 62   |
| `setMediaStatusConditionally` | `'METADATA'`  | 110  |
| **`setMediaStatus`**          | **`'metadata'`** | **155** |
| `deleteMedia`                 | `'METADATA'`  | 186  |

### CloudWatch Log Evidence (Before Fix)
The three DynamoDB log lines during a resize operation revealed the mismatch:
1. `{"sk":"METADATA"}` — `getMedia` reads the record (uppercase)
2. `{"newStatus":"PROCESSING","sk":"metadata"}` — `setMediaStatus` writes to WRONG item (lowercase)
3. `{"newStatus":"COMPLETE","sk":"metadata"}` — `setMediaStatus` writes to WRONG item (lowercase)

### Code Diff
```diff
       PK: { S: `MEDIA#${mediaId}` },
-      SK: { S: 'metadata' },
+      SK: { S: 'METADATA' },
     },

-    logger.info({ mediaId, sk: 'metadata', newStatus }, 'Updating media status in DynamoDB');
+    logger.info({ mediaId, sk: 'METADATA', newStatus }, 'Updating media status in DynamoDB');
```

### Verification
After fix, CloudWatch logs show consistent SK across all operations:
```
{"sk":"METADATA"} — getMedia (read)
{"newStatus":"PROCESSING","sk":"METADATA"} — setMediaStatus (write)
{"newStatus":"COMPLETE","sk":"METADATA"} — setMediaStatus (write)
```

Full end-to-end test:
```
$ curl -X POST "http://<alb-dns>/v1/media/upload?width=500" -F "file=@sample.jpg"
{"mediaId":"a4e5961f-f850-472e-913a-e803c821747d"}

$ curl -X PUT "http://<alb-dns>/v1/media/a4e5961f/resize?width=500"
{"mediaId":"a4e5961f-f850-472e-913a-e803c821747d","status":"COMPLETE"}

$ curl http://<alb-dns>/v1/media/a4e5961f/status
{"status":"COMPLETE"}

$ curl -i http://<alb-dns>/v1/media/a4e5961f/download
HTTP/1.1 302 Found
Location: https://hummingbird-production-....s3.us-west-2.amazonaws.com/uploads/...
```

---

## Full Git Diff (All Changes)

```diff
diff --git a/server.js b/server.js
--- a/server.js
+++ b/server.js
@@ -32,7 +32,7 @@

 app.use('/v1/media', mediaRoutes);

-const port = process.env.APP_PORT;
+const port = process.env.APP_PORT || 9000;

 app.listen(port, () => {
   logger.info(`Example app listening on port ${port}`);

diff --git a/clients/dynamodb.js b/clients/dynamodb.js
--- a/clients/dynamodb.js
+++ b/clients/dynamodb.js
@@ -81,6 +81,7 @@
       name: Item.name.S,
       mimetype: Item.mimetype.S,
       status: Item.status.S,
+      width: Number(Item.width.N),
     };
   } catch (error) {
     logger.error(error);
@@ -151,7 +152,7 @@
     Key: {
       PK: { S: `MEDIA#${mediaId}` },
-      SK: { S: 'metadata' },
+      SK: { S: 'METADATA' },
     },
@@ -163,7 +164,7 @@

-    logger.info({ mediaId, sk: 'metadata', newStatus }, 'Updating media status in DynamoDB');
+    logger.info({ mediaId, sk: 'METADATA', newStatus }, 'Updating media status in DynamoDB');

diff --git a/controllers/media.js b/controllers/media.js
--- a/controllers/media.js
+++ b/controllers/media.js
@@ -108,10 +108,10 @@
-    if (media.status !== MEDIA_STATUS.PROCESSING) {
+    if (media.status !== MEDIA_STATUS.COMPLETE) {
       const SIXTY_SECONDS = 60;
       res.set('Retry-After', SIXTY_SECONDS);
-      res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);
+      res.set('Location', `http://${req.hostname}/v1/media/${mediaId}/status`);
```
