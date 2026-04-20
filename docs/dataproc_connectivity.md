# Dataproc Connectivity in MiniSky

MiniSky provides high-fidelity emulators for Google Cloud Storage (GCS) and BigQuery. The Dataproc emulator is pre-configured to connect to these services automatically.

## Automatic Configuration

When you create a Dataproc cluster in MiniSky, the following Hadoop properties are automatically injected into all nodes:

| Property | Value | Description |
|----------|-------|-------------|
| `fs.gs.impl` | `com.google.cloud.hadoop.fs.gcs.GoogleHadoopFileSystem` | Uses the GCS connector. |
| `google.cloud.auth.service.account.enable` | `false` | Disables authentication for local testing. |
| `fs.gs.endpoint` | `http://minisky-gcs:4443` | Points to the internal MiniSky GCS emulator. |
| `BIGQUERY_REST_ENDPOINT` | `http://host.docker.internal:8080/bigquery/v2` | Points to the internal BigQuery shim. |

## Running Jobs

### PySpark example

You can submit jobs via the MiniSky Dashboard. For a PySpark job, you don't need to specify extra configuration to reach GCS.

```python
from pyspark.sql import SparkSession

spark = SparkSession.builder.appName("MiniSkyTest").get()

# This will automatically point to the MiniSky GCS emulator
df = spark.read.text("gs://my-bucket/data.txt")
print(f"Number of lines: {df.count()}")
```

### BigQuery example

To reach the BigQuery shim, ensure you have the `spark-bigquery-with-dependencies` JAR available (Bitnami Spark 4.0 images often include common connectors, but you may need to add it to `jarFileUris` in your job submission).

```python
df = spark.read.format("bigquery") \
    .option("project", "local-dev-project") \
    .option("dataset", "my_dataset") \
    .option("table", "my_table") \
    .load()
```

## Internal Network Details

All containers (Spark Master, Spark Workers, Cloud SQL, GCS Emulator) run inside the `minisky-net` Docker bridge network. 

- **Accessing the GCS emulator**: Use the hostname `minisky-gcs`.
- **Accessing the BigQuery shim**: Use the hostname `host.docker.internal` (Standard on Docker Desktop) or the gateway IP (usually `172.18.0.1`).
