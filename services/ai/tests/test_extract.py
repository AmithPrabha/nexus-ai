from pathlib import Path

import pytest

from app.extract import DocumentError, MockProvider, extract, fixture

ROOT = (
    Path(__file__).resolve().parents[3]
    if len(Path(__file__).resolve().parents) > 3
    else Path("/")
)
SAMPLES = ROOT / "fixtures/invoices"


def test_five_pdfs_have_verified_evidence():
    pdfs = list(SAMPLES.glob("*.pdf"))
    assert len(pdfs) == 5
    for path in pdfs:
        result = extract(path)
        assert result.fields["invoice_number"].startswith("NX-")
        assert result.evidence["total"].page == 1 and not result.warnings
    assert extract(SAMPLES / "courier.pdf").fields["supplier_trn"] is None


def test_unverified_evidence():
    class WrongExcerpt(MockProvider):
        def extract(self, pages):
            data = super().extract(pages)
            data["evidence"]["total"]["excerpt"] = "Invented quotation"
            return data

    result = extract(SAMPLES / "equipment.pdf", WrongExcerpt())
    assert "total" not in result.evidence
    assert "Unverified evidence for total" in result.warnings


def test_invalid_schema_retried_once():
    class Invalid:
        calls = 0

        def extract(self, pages):
            self.calls += 1
            return {}

    provider = Invalid()
    with pytest.raises(DocumentError, match="SCHEMA_ERROR"):
        extract(SAMPLES / "courier.pdf", provider)
    assert provider.calls == 2


def test_unknown_sample():
    with pytest.raises(DocumentError):
        fixture("../secret")
