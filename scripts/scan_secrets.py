"""Scan the files eligible for release, including uncommitted source, with redacted output."""
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
paths = subprocess.check_output(
    ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=root
).decode().split("\0")
with tempfile.TemporaryDirectory(prefix="nexusai-scan-") as temp:
    for name in filter(None, paths):
        path = root / name
        if not path.exists():
            continue
        if not path.resolve().is_relative_to(root) or path.name == ".env":
            raise SystemExit("A private or external path was included in release sources")
        dest = Path(temp) / name
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, dest)
    subprocess.run(["docker", "run", "--rm", "--network=none", "-v",
                    f"{temp}:/release:ro", "ghcr.io/gitleaks/gitleaks:v8.24.3",
                    "dir", "/release", "--redact", "--no-banner"], check=True)
