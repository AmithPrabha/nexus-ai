"""Trusted operator CLI. Use the migration administrator's connection."""

import argparse
import hashlib
import os
import secrets
from uuid import UUID

import psycopg


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    tenant = actions.add_parser("create-company")
    tenant.add_argument("name")
    user = actions.add_parser("create-user")
    user.add_argument("subject", help="Stable operator-verified account identifier")
    user.add_argument("display_name")
    for name in ("grant", "revoke", "issue"):
        command = actions.add_parser(name)
        command.add_argument("tenant", type=UUID)
        command.add_argument("user", type=UUID)
        if name == "grant":
            command.add_argument(
                "role", choices=["OWNER", "ADMIN", "REVIEWER", "VIEWER"]
            )
    args = parser.parse_args()
    with psycopg.connect(os.environ["SEED_DB"]) as db:
        if args.action == "create-company":
            result = db.execute(
                "INSERT INTO tenants(kind,name) VALUES('COMPANY',%s) RETURNING id",
                (args.name,),
            ).fetchone()[0]
        elif args.action == "create-user":
            result = db.execute(
                "INSERT INTO users(subject,display_name) VALUES(%s,%s) RETURNING id",
                (args.subject, args.display_name),
            ).fetchone()[0]
        elif args.action == "grant":
            db.execute(
                """INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES(%s,%s,%s)
                ON CONFLICT(tenant_id,user_id) DO UPDATE SET role=excluded.role,status='ACTIVE'""",
                (args.tenant, args.user, args.role),
            )
            result = "Membership active."
        elif args.action == "revoke":
            changed = db.execute(
                "UPDATE tenant_memberships SET status='REVOKED' WHERE tenant_id=%s AND user_id=%s",
                (args.tenant, args.user),
            ).rowcount
            if not changed:
                raise SystemExit("Membership not found.")
            result = "Membership revoked."
        else:
            result = secrets.token_urlsafe(32)
            membership = db.execute(
                """SELECT 1 FROM tenant_memberships WHERE tenant_id=%s AND user_id=%s
                AND status='ACTIVE' FOR SHARE""",
                (args.tenant, args.user),
            ).fetchone()
            if not membership:
                raise SystemExit("An active membership is required.")
            db.execute(
                "INSERT INTO login_codes(digest,tenant_id,user_id) VALUES(%s,%s,%s)",
                (hashlib.sha256(result.encode()).digest(), args.tenant, args.user),
            )
    # The code is shown once to the issuing operator after the transaction commits.
    print(result)


if __name__ == "__main__":
    main()
