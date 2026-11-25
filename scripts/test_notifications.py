"""Verify durable delivery, duplicate receipts and signed/replayed requests on local fixtures."""

import base64
import hashlib
import hmac
import json
import os
import time

import httpx
import psycopg

from smoke import Client
from test_invoices import create, until_ready

client = Client()
client.start()
item = until_ready(client, create(client, "maintenance"))
invoice = item["invoice"]["invoice_id"]
with psycopg.connect(os.environ["TEST_DB"], autocommit=True) as db:
    for _ in range(40):
        event, state = db.execute(
            "SELECT id,delivery_state FROM sandbox_notifications WHERE invoice_id=%s",
            (invoice,),
        ).fetchone()
        if state == "RECORDED":
            break
        time.sleep(0.5)
    assert state == "RECORDED", state

    def envelope(timestamp):
        body = {
            "schema_version": 1,
            "event_id": str(event),
            "invoice_id": invoice,
            "timestamp": timestamp,
            "demo": True,
            "state": "PENDING_REVIEW",
            "reason_codes": ["TRN_FORMAT"],
        }
        payload = (
            base64.urlsafe_b64encode(json.dumps(body).encode()).decode().rstrip("=")
        )
        signature = hmac.new(
            os.environ["SERVICE_TOKEN"].encode(),
            ("notification.v1." + payload).encode(),
            hashlib.sha256,
        ).hexdigest()
        return {"payload": payload, "signature": signature}

    url = "http://gateway:8081/internal/notifications/receive"
    signed = envelope(int(time.time()))
    assert httpx.post(url, json=signed).json() == {"event_id": str(event)}
    assert httpx.post(url, json={**signed, "signature": ""}).status_code == 403
    assert httpx.post(url, json=envelope(int(time.time()) - 301)).status_code == 403
    assert db.execute(
        "SELECT count(*) FROM sandbox_receipts WHERE event_id=%s", (event,)
    ).fetchone() == (1,)
    assert (
        client.request("GET", f"/api/v1/invoices/{invoice}")["sandbox_notification"]
        == "RECORDED"
    )
print(
    "Fictional review event delivered, signed receipt deduplicated, stale and modified envelopes rejected"
)
