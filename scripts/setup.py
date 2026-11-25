"""Generate private local Compose configuration once."""

import os
import secrets
from pathlib import Path

path = Path(__file__).resolve().parents[1] / ".env"
try:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    print("Using existing .env")
else:
    with os.fdopen(fd, "w") as out:
        for name in (
            "POSTGRES_PASSWORD",
            "GATEWAY_PASSWORD",
            "POLICY_PASSWORD",
            "SERVICE_TOKEN",
        ):
            print(f"{name}={secrets.token_hex(32)}", file=out)
        print("PUBLIC_ORIGIN=http://localhost:3000", file=out)
    print("Created local .env with mode 0600")
