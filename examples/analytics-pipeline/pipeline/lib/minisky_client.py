"""Thin REST client for the MiniSky-emulated Google Cloud APIs.

The Airflow tasks deliberately speak raw REST rather than importing
``google-cloud-*``: it keeps the Airflow virtualenv free of the dependency tree
that dbt pins, and it makes every call the pipeline issues visible in
``minisky logs tail``.

Endpoints used (all path-routed by MiniSky's gateway, pkg/router/proxy.go):

    Cloud Storage   /storage/v1/b/...            /upload/storage/v1/b/...
    BigQuery        /bigquery/v2/projects/...
    Pub/Sub         /v1/projects/{p}/topics/...
"""

from __future__ import annotations

import csv
import io
import json
import os
import time
from base64 import b64encode
from dataclasses import dataclass
from typing import Any, Iterable, Sequence
from urllib.parse import quote

import requests

DEFAULT_TIMEOUT = 60


class MiniSkyError(RuntimeError):
    """Raised when the emulator returns an error or an unusable response."""


@dataclass
class MiniSky:
    """Client bound to one MiniSky gateway and project."""

    endpoint: str
    project: str
    token: str = "minisky-local-token"
    timeout: int = DEFAULT_TIMEOUT

    @classmethod
    def from_env(cls) -> "MiniSky":
        endpoint = os.environ.get("MINISKY_ENDPOINT")
        project = os.environ.get("MINISKY_PROJECT")
        if not endpoint or not project:
            raise MiniSkyError(
                "MINISKY_ENDPOINT and MINISKY_PROJECT must be set "
                "(they come from /opt/pipeline/env.sh)"
            )
        return cls(
            endpoint=endpoint.rstrip("/"),
            project=project,
            token=os.environ.get("MINISKY_ACCESS_TOKEN", "minisky-local-token"),
        )

    # ── plumbing ────────────────────────────────────────────────────────────
    def _request(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        url = f"{self.endpoint}{path}"
        headers = kwargs.pop("headers", {})
        headers.setdefault("Authorization", f"Bearer {self.token}")
        response = requests.request(
            method, url, headers=headers, timeout=self.timeout, **kwargs
        )
        if response.status_code >= 400:
            raise MiniSkyError(
                f"{method} {url} -> {response.status_code}: {response.text[:500]}"
            )
        return response

    # ── Cloud Storage ───────────────────────────────────────────────────────
    def gcs_download(self, bucket: str, obj: str) -> bytes:
        path = f"/storage/v1/b/{bucket}/o/{quote(obj, safe='')}?alt=media"
        return self._request("GET", path).content

    def gcs_upload(
        self, bucket: str, obj: str, data: bytes, content_type: str = "text/csv"
    ) -> dict:
        path = (
            f"/upload/storage/v1/b/{bucket}/o"
            f"?uploadType=media&name={quote(obj, safe='')}"
        )
        response = self._request(
            "POST", path, data=data, headers={"Content-Type": content_type}
        )
        return response.json() if response.content else {}

    def gcs_list(self, bucket: str, prefix: str = "") -> list[dict]:
        path = f"/storage/v1/b/{bucket}/o"
        if prefix:
            path += f"?prefix={quote(prefix, safe='')}"
        return self._request("GET", path).json().get("items", []) or []

    # ── BigQuery ────────────────────────────────────────────────────────────
    def bq_query(self, sql: str, poll_seconds: float = 0.5, max_wait: int = 300) -> list[dict]:
        """Run a query job to completion and return its rows as dicts."""
        body = {
            "configuration": {"query": {"query": sql, "useLegacySql": False}},
            "jobReference": {"projectId": self.project, "location": "US"},
        }
        job = self._request(
            "POST", f"/bigquery/v2/projects/{self.project}/jobs", json=body
        ).json()
        job_id = job.get("jobReference", {}).get("jobId")
        if not job_id:
            raise MiniSkyError(f"job insert returned no jobId: {json.dumps(job)[:300]}")

        deadline = time.time() + max_wait
        while time.time() < deadline:
            status = self._request(
                "GET", f"/bigquery/v2/projects/{self.project}/jobs/{job_id}"
            ).json()
            state = status.get("status", {}).get("state")
            if state == "DONE":
                error = status.get("status", {}).get("errorResult")
                if error:
                    raise MiniSkyError(
                        f"query failed: {error.get('message', error)}\nSQL: {sql[:400]}"
                    )
                return self._bq_results(job_id)
            time.sleep(poll_seconds)
        raise MiniSkyError(f"query job {job_id} did not finish within {max_wait}s")

    def _bq_results(self, job_id: str) -> list[dict]:
        payload = self._request(
            "GET", f"/bigquery/v2/projects/{self.project}/jobs/{job_id}/results"
        ).json()
        fields = [f["name"] for f in payload.get("schema", {}).get("fields", [])]
        rows = []
        for row in payload.get("rows", []) or []:
            values = [cell.get("v") for cell in row.get("f", [])]
            rows.append(dict(zip(fields, values)))
        return rows

    def bq_execute(self, sql: str) -> None:
        """Run a statement whose rows are not needed (DDL/DML)."""
        self.bq_query(sql)

    def bq_table_exists(self, dataset: str, table: str) -> bool:
        try:
            self._request(
                "GET",
                f"/bigquery/v2/projects/{self.project}/datasets/{dataset}/tables/{table}",
            )
            return True
        except MiniSkyError:
            return False

    # ── Pub/Sub ─────────────────────────────────────────────────────────────
    def publish(self, topic: str, payload: dict, attributes: dict | None = None) -> list[str]:
        body = {
            "messages": [
                {
                    "data": b64encode(json.dumps(payload).encode()).decode(),
                    "attributes": {k: str(v) for k, v in (attributes or {}).items()},
                }
            ]
        }
        response = self._request(
            "POST", f"/v1/projects/{self.project}/topics/{topic}:publish", json=body
        ).json()
        return response.get("messageIds", [])


# ── SQL helpers ─────────────────────────────────────────────────────────────

def sql_literal(value: Any) -> str:
    """Render a Python value as a BigQuery SQL literal."""
    if value is None or value == "":
        return "NULL"
    if isinstance(value, bool):
        return "TRUE" if value else "FALSE"
    if isinstance(value, (int, float)):
        return repr(value)
    return "'" + str(value).replace("\\", "\\\\").replace("'", "\\'") + "'"


def rows_to_insert(dataset: str, table: str, columns: Sequence[str],
                   rows: Iterable[Sequence[Any]], batch_size: int = 200) -> list[str]:
    """Build batched multi-row INSERT statements for a table."""
    statements: list[str] = []
    batch: list[str] = []
    column_list = ", ".join(columns)

    for row in rows:
        batch.append("(" + ", ".join(sql_literal(v) for v in row) + ")")
        if len(batch) >= batch_size:
            statements.append(
                f"INSERT INTO {dataset}.{table} ({column_list}) VALUES\n"
                + ",\n".join(batch)
            )
            batch = []

    if batch:
        statements.append(
            f"INSERT INTO {dataset}.{table} ({column_list}) VALUES\n" + ",\n".join(batch)
        )
    return statements


def parse_csv(data: bytes) -> tuple[list[str], list[list[str]]]:
    """Parse CSV bytes into (header, rows)."""
    reader = csv.reader(io.StringIO(data.decode("utf-8")))
    header = next(reader)
    return header, [row for row in reader if row]


def coerce(value: str, kind: str) -> Any:
    """Coerce a CSV string into the type the raw table declares."""
    if value == "":
        return None
    if kind == "int":
        return int(value)
    if kind == "float":
        return float(value)
    return value
