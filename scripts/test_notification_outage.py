"""Run while the optional workflow service is offline, with the gateway in n8n mode."""

import time

from smoke import Client
from test_invoices import create, until_ready

client = Client()
client.start()
invoice = until_ready(client, create(client, "courier"))["invoice"]
path = "/api/v1/invoices/" + invoice["invoice_id"]
assert invoice["status"] == "PENDING_REVIEW"
for _ in range(60):
    detail = client.request("GET", path)
    if detail["sandbox_notification"] == "FAILED":
        break
    time.sleep(0.5)
else:
    raise AssertionError("Expected bounded sandbox delivery failure")
assert detail["invoice"]["status"] == "PENDING_REVIEW"
assert (
    client.request(
        "POST",
        path + "/decision",
        200,
        {
            "version": detail["version"],
            "decision": "reject",
            "reason": "Missing registration number",
        },
    )["status"]
    == "REJECTED"
)
print(
    "Notification retries exhausted independently; invoice remained reviewable and accepted the human decision"
)
