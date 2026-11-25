"""Exercise the public gateway through its HTTP entry point."""

import http.cookiejar
import json
import os
import urllib.error
import urllib.request

BASE = os.getenv("GATEWAY_URL", "http://127.0.0.1:8080")
ORIGIN = os.getenv("PUBLIC_ORIGIN", "http://localhost:3000")


class Client:
    def __init__(self):
        self.http = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
        )
        self.csrf = ""

    def request(self, method, path, status=200, body=None, **headers):
        req = urllib.request.Request(
            BASE + path,
            method=method,
            data=json.dumps(body).encode() if body is not None else None,
            headers={
                "Origin": ORIGIN,
                "Content-Type": "application/json",
                "X-CSRF-Token": self.csrf,
                **headers,
            },
        )
        try:
            response = self.http.open(req, timeout=10)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            result = json.load(response)
            assert response.status == status, (method, path, response.status, result)
            assert response.headers["X-Request-ID"]
            return result

    def start(self):
        state = self.request("POST", "/api/v1/demo/sessions", 201, {})
        self.csrf = state["csrf_token"]
        return state["session_id"]


if __name__ == "__main__":
    first, second = Client(), Client()
    assert first.request("GET", "/healthz")["status"] == "healthy"
    assert first.request("GET", "/readyz")["status"] == "ready"
    first.request("GET", "/api/v1/samples", 401)
    first.request(
        "POST", "/api/v1/demo/sessions", 403, {}, Origin="https://invalid.example"
    )
    a = first.start()
    assert a == first.start() and a != second.start()
    samples = first.request("GET", "/api/v1/samples")["items"]
    assert len(samples) == 5
    for sample in samples:
        with first.http.open(BASE + sample["preview_url"], timeout=5) as response:
            assert response.headers.get_content_type() == "application/pdf"
            assert response.read(5) == b"%PDF-"
    first.request("POST", "/internal/jobs/invalid/complete", 404, {})
    first.request("POST", "/api/v1/demo/reset", 200, {})
    first.request("GET", "/api/v1/samples", 401)
    assert first.start() != a
    print("Foundation HTTP checks passed")
