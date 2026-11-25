"""Exercise named sign-in, real PostgreSQL roles, Redis jobs and HTTP authorization."""

import hashlib
import json
import os
import secrets
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from threading import Barrier
from uuid import uuid4

import httpx
import psycopg
from smoke import BASE, ORIGIN, Client
from test_invoices import V1, create, until_ready


def code(db, tenant, user, expired=False):
    raw = secrets.token_urlsafe(32)
    db.execute(
        """INSERT INTO login_codes(digest,tenant_id,user_id,expires_at)
        VALUES(%s,%s,%s,now()+%s::interval)""",
        (
            hashlib.sha256(raw.encode()).digest(),
            tenant,
            user,
            "-1 minute" if expired else "10 minutes",
        ),
    )
    return raw


def login(raw, expected=201):
    client = Client()
    result = client.request("POST", V1 + "/auth/login", expected, {"code": raw})
    if expected == 201:
        client.csrf = result["csrf_token"]
    return client, result


def suite():
    with psycopg.connect(os.environ["TEST_DB"], autocommit=True) as db:

        def company(name):
            return db.execute(
                "INSERT INTO tenants(kind,name) VALUES('COMPANY',%s) RETURNING id",
                (name,),
            ).fetchone()[0]

        def user(tenant, name, role):
            uid = db.execute(
                "INSERT INTO users(subject,display_name) VALUES(%s,%s) RETURNING id",
                (str(uuid4()), name),
            ).fetchone()[0]
            db.execute(
                "INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES(%s,%s,%s)",
                (tenant, uid, role),
            )
            return uid

        a, b = company("Company A"), company("Company B")
        reviewer = user(a, "Ada Reviewer", "REVIEWER")
        viewer = user(a, "Val Viewer", "VIEWER")
        owner = user(a, "Ari Owner", "OWNER")
        admin_b = user(b, "Bea Admin", "ADMIN")
        submitted_code = code(db, a, reviewer)
        alice, session = login(submitted_code)
        assert session["member"] == {
            "tenant_id": str(a),
            "tenant_name": "Company A",
            "user_id": str(reviewer),
            "display_name": "Ada Reviewer",
            "role": "REVIEWER",
        }
        login(submitted_code, 401)
        login(code(db, a, reviewer, expired=True), 401)
        bob, _ = login(code(db, b, admin_b))
        val, _ = login(code(db, a, viewer))
        ari, _ = login(code(db, a, owner))
        queued = create(alice, "office-supplies")
        detail = until_ready(alice, queued)
        path = V1 + "/invoices/" + queued["invoice_id"]
        job = V1 + "/jobs/" + queued["job_id"]
        assert detail["invoice"]["status"] == "READY_FOR_APPROVAL"
        version = detail["version"]
        approve = {
            "version": version,
            "decision": "approve",
            "reason": "Source checked",
        }
        patch = {"version": version, "reason": "Rechecked", "fields": {"total": "1.00"}}
        assert val.request("GET", path)["version"] == version
        assert bob.request("GET", V1 + "/invoices")["items"] == []
        assert bob.request("GET", V1 + "/metrics/demo")["invoices"] == 0
        for suffix in (path, path + "/audit", job):
            bob.request("GET", suffix, 404)
        bob.request("PATCH", path + "/review", 404, patch)
        bob.request("POST", path + "/decision", 404, approve)
        bob.request(
            "POST",
            path + "/retry",
            404,
            {"version": version},
            **{"Idempotency-Key": "other-tenant"},
        )
        val.request("POST", path + "/decision", 403, approve)
        val.request("PATCH", path + "/review", 403, patch)
        alice.request("POST", path + "/decision", 403, approve, **{"X-CSRF-Token": ""})
        alice.request(
            "POST", path + "/decision", 400, {**approve, "actor": "Someone else"}
        )
        alice.request("POST", V1 + "/demo/reset", 403, {})
        # One user may belong to multiple companies. A code fixes the active membership.
        db.execute(
            "INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES(%s,%s,'VIEWER')",
            (b, reviewer),
        )
        other_org, other_session = login(code(db, b, reviewer))
        assert other_session["member"]["tenant_id"] == str(b)
        other_org.request("GET", path, 404)

        # Race independent real sessions for the same named reviewer.
        second, _ = login(code(db, a, reviewer))
        barrier = Barrier(2)

        def decide(args):
            client, decision = args
            req = client.http

            barrier.wait(timeout=5)
            request = urllib.request.Request(
                BASE + path + "/decision",
                method="POST",
                data=json.dumps({**approve, "decision": decision}).encode(),
                headers={
                    "Content-Type": "application/json",
                    "Origin": ORIGIN,
                    "X-CSRF-Token": client.csrf,
                },
            )
            try:
                response = req.open(request, timeout=10)
            except urllib.error.HTTPError as e:
                response = e
            with response:
                return response.status, decision

        with ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(decide, [(alice, "approve"), (second, "reject")]))
        assert sorted(status for status, _ in results) == [200, 409], results
        winner = next(action for status, action in results if status == 200)
        events = [
            e
            for e in alice.request("GET", path + "/audit")["items"]
            if e["action"] in ("approve", "reject")
        ]
        assert len(events) == 1 and events[0]["action"] == winner
        assert (
            events[0]["actor_user_id"] == str(reviewer)
            and events[0]["actor"] == "Ada Reviewer"
        )

        # Guaranteed clean approval as the named reviewer, independent of the race winner.
        approved_job = create(alice, "maintenance")
        pending = until_ready(alice, approved_job)
        approved_path = V1 + "/invoices/" + approved_job["invoice_id"]
        assert pending["invoice"]["status"] == "PENDING_REVIEW"
        alice.request(
            "POST",
            approved_path + "/decision",
            403,
            {**approve, "version": pending["version"]},
        )
        fixed = alice.request(
            "PATCH",
            approved_path + "/review",
            200,
            {
                "version": pending["version"],
                "fields": {"supplier_trn": "100000000000987"},
                "reason": "Checked supplier record",
            },
        )
        assert fixed["invoice"]["status"] == "READY_FOR_APPROVAL", fixed[
            "validation_results"
        ]
        accepted = alice.request(
            "POST",
            approved_path + "/decision",
            200,
            {**approve, "version": fixed["version"]},
        )
        assert accepted["status"] == "APPROVED"
        event = alice.request("GET", approved_path + "/audit")["items"][-1]
        assert (
            event["tenant_id"],
            event["actor_user_id"],
            event["actor_role"],
            event["actor"],
            event["action"],
        ) == (str(a), str(reviewer), "REVIEWER", "Ada Reviewer", "approve")
        assert (
            event["invoice_version"] == accepted["version"]
            and event["revision_no"] == 2
            and event["ruleset"] == "aed-demo-v1"
        )
        pending2 = create(alice, "equipment")
        high = until_ready(alice, pending2)
        ari.request(
            "POST",
            V1 + "/invoices/" + pending2["invoice_id"] + "/decision",
            200,
            {
                **approve,
                "version": high["version"],
                "reason": "Owner reviewed threshold",
            },
        )

        demo = Client()
        demo_id = demo.start()
        demo.request("GET", approved_path, 404)
        demo.request("POST", V1 + "/demo/reset", 200, {})
        # Run the same purge predicate against this test's own expired demo session.
        db.execute(
            "DELETE FROM workspaces WHERE id=%s AND expires_at<=now()", (demo_id,)
        )
        alice.request("POST", V1 + "/auth/logout", 200)
        alice.request("GET", approved_path, 401)
        new_session, _ = login(code(db, a, reviewer))
        assert (
            new_session.request("GET", approved_path)["invoice"]["status"] == "APPROVED"
        )
        assert (
            new_session.request("GET", approved_path + "/audit")["items"][-1] == event
        )
        with db.transaction():
            db.execute("SET LOCAL ROLE nexus_request")
            db.execute("SELECT set_config('nexus.tenant',%s,true)", (str(b),))
            for table, key, val_id in (
                ("invoices", "id", queued["invoice_id"]),
                ("jobs", "id", queued["job_id"]),
                ("invoice_revisions", "invoice_id", queued["invoice_id"]),
                ("audit_events", "invoice_id", queued["invoice_id"]),
            ):
                # Identifiers come only from the fixed test tuple above.
                assert (
                    db.execute(
                        f"SELECT count(*) FROM {table} WHERE {key}=%s", (val_id,)
                    ).fetchone()[0]
                    == 0
                )
        with db.transaction():
            db.execute("SET LOCAL ROLE nexus_request")
            assert db.execute("SELECT count(*) FROM invoices").fetchone()[0] == 0
        policy = db.execute("SELECT id FROM policies ORDER BY id LIMIT 1").fetchone()[0]
        db.execute("INSERT INTO workspace_policies VALUES(%s,%s)", (a, policy))
        with db.transaction():
            db.execute("SET LOCAL ROLE nexus_policy")
            db.execute("SELECT set_config('nexus.tenant',%s,true)", (str(a),))
            assert db.execute("SELECT count(*) FROM policy_chunks").fetchone()[0] > 0
            db.execute("SELECT set_config('nexus.tenant',%s,true)", (str(b),))
            assert db.execute("SELECT count(*) FROM policy_chunks").fetchone()[0] == 0
        assert not db.execute(
            "SELECT has_column_privilege('nexus_gateway','login_codes','digest','SELECT')"
        ).fetchone()[0]
        # The storage boundary rejects a valid user UUID from a different company.
        try:
            with db.transaction(force_rollback=True):
                db.execute(
                    """INSERT INTO invoice_revisions(invoice_id,revision_no,fields,evidence,warnings,
                    validation_results,actor,actor_user_id)
                    SELECT invoice_id,revision_no+1000,fields,evidence,warnings,validation_results,
                    'Fixture author',%s FROM invoice_revisions WHERE invoice_id=%s AND revision_no=1""",
                    (admin_b, queued["invoice_id"]),
                )
        except psycopg.errors.ForeignKeyViolation:
            pass
        else:
            raise AssertionError(
                "Database accepted a revision author from another company"
            )
        # Expiry, revocation and reactivation all require a fresh credential.
        db.execute(
            "UPDATE user_sessions SET expires_at=now()-interval '1 minute' WHERE tenant_id=%s AND user_id=%s",
            (a, reviewer),
        )
        new_session.request("GET", approved_path, 401)
        signed, _ = login(code(db, a, reviewer))
        outstanding = code(db, a, reviewer)
        db.execute(
            "UPDATE tenant_memberships SET status='REVOKED' WHERE tenant_id=%s AND user_id=%s",
            (a, reviewer),
        )
        signed.request("GET", approved_path, 401)
        signed.request("POST", approved_path + "/decision", 401, approve)
        other_org.request("GET", V1 + "/auth/session", 200)
        db.execute(
            "UPDATE tenant_memberships SET status='ACTIVE' WHERE tenant_id=%s AND user_id=%s",
            (a, reviewer),
        )
        signed.request("GET", approved_path, 401)
        login(outstanding, 401)
        assert ari.request("GET", approved_path + "/audit")["items"][-1] == event
        assert httpx.get(BASE + "/readyz").status_code == 200
    print(
        "Named approval, two-company isolation, role checks, concurrent decisions, expiry, revocation, SQL policies and durable history passed"
    )


if __name__ == "__main__":
    suite()
