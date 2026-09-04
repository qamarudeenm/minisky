# Cloud Storage Event Triggers

MiniSky supports automated event triggers for Cloud Functions based on Storage bucket activity.

## Supported Events
- `google.storage.object.finalize`: Triggered when a new object is created or an existing object is overwritten.
- `google.storage.object.delete`: Triggered when an object is deleted.

## Usage Guide

### 1. Function Signature
Your Python function should use the `(event, context)` signature for background events.

```python
def gcs_trigger(event, context):
    """
    Args:
        event (dict): Event payload.
        context (google.cloud.functions.Context): Metadata for the event.
    """
    bucket = event['bucket']
    name = event['name']
    print(f"File {name} processed in bucket {bucket}")
```

### 2. Deployment
When deploying via the MiniSky Dashboard, ensure you provide the `eventTrigger` configuration in your deployment request.

**Example Deployment Payload:**
```json
{
  "name": "my-gcs-function",
  "runtime": "python312",
  "entryPoint": "gcs_trigger",
  "eventTrigger": {
    "eventType": "google.storage.object.finalize",
    "resource": "projects/_/buckets/my-test-bucket"
  }
}
```

### 3. Testing with MiniSky
1. Deploy your function using the **Serverless Console**.
2. Go to the **Data Storage Buckets** page.
3. Create a bucket named `my-test-bucket`.
4. Upload any file to that bucket.
5. Check the **Cloud Logging** page. You should see:
   - `[Storage Event] File finalized: gs://my-test-bucket/your-file.txt`
   - `[Serverless] 🎯 Triggering function: my-gcs-function`
   - `[Serverless] ✅ Trigger success`

## Payload Structure
The `event` dictionary passed to your function contains:
- `bucket`: Name of the bucket.
- `name`: Name of the object.
- `contentType`: MIME type (defaults to `application/octet-stream` in emulator).
- `timeCreated`: RFC3339 timestamp.
- `updated`: RFC3339 timestamp.
