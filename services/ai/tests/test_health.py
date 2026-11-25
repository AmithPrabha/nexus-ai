from fastapi.testclient import TestClient

from app.api import api


def test_health_labels_mock():
    assert TestClient(api).get("/health").json() == {
        "status": "healthy",
        "mode": "mock",
    }


def test_checked_in_contract_matches_fastapi():
    import json
    from pathlib import Path

    here = Path(__file__).resolve()
    root = next(
        parent for parent in here.parents if (parent / "packages/contracts").is_dir()
    )
    assert (
        json.loads((root / "packages/contracts/ai.openapi.json").read_text())
        == api.openapi()
    )
