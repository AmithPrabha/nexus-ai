import hashlib
import json
import os
import uuid
from pathlib import Path

import psycopg

from app.retrieve import VERSION, embedding


def seed():
    policies = json.loads(Path("/fixtures/policies/catalog.json").read_text())
    with psycopg.connect(os.environ["SEED_DB"]) as db:
        db.execute("SELECT pg_advisory_xact_lock(615231)")
        db.execute("LOCK TABLE workspaces IN SHARE ROW EXCLUSIVE MODE")
        existing = {row[0] for row in db.execute("SELECT id FROM policies")}
        for p in policies:
            digest = hashlib.sha256(json.dumps(p, sort_keys=True).encode()).hexdigest()
            db.execute(
                """INSERT INTO policies VALUES (%s,%s,%s,%s) ON CONFLICT(id) DO UPDATE
                SET title=EXCLUDED.title,version=EXCLUDED.version,source_hash=EXCLUDED.source_hash""",
                (p["id"], p["title"], p["version"], digest),
            )
            db.execute("DELETE FROM policy_chunks WHERE policy_id=%s", (p["id"],))
            for c in p["clauses"]:
                chunk = uuid.uuid5(
                    uuid.NAMESPACE_URL, f"nexusai/{p['id']}/{digest}/{c['clause']}"
                )
                db.execute(
                    """INSERT INTO policy_chunks(id,policy_id,page,clause,body,topic,embedding,embedding_version)
                    VALUES (%s,%s,%s,%s,%s,%s,%s::vector,%s)""",
                    (
                        chunk,
                        p["id"],
                        c["page"],
                        c["clause"],
                        c["text"],
                        c["topic"],
                        embedding(c["topic"] + " " + c["text"]),
                        VERSION,
                    ),
                )
            if p["id"] not in existing:
                db.execute(
                    "INSERT INTO workspace_policies SELECT id,%s FROM workspaces",
                    (p["id"],),
                )
    print(f"Indexed {len(policies)} fictional policies")


if __name__ == "__main__":
    seed()
