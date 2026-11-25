import os
import re
from pathlib import Path
from typing import Protocol

import pymupdf
from pydantic import BaseModel, ConfigDict, Field, ValidationError

LABELS = {
    "Supplier": "supplier_name",
    "Supplier TRN": "supplier_trn",
    "Invoice": "invoice_number",
    "Date": "invoice_date",
    "Currency": "currency",
    "Subtotal": "subtotal",
    "VAT rate": "vat_rate",
    "VAT amount": "vat_amount",
    "Total": "total",
    "Treatment": "tax_treatment",
}


class DocumentError(Exception):
    pass


class Evidence(BaseModel):
    model_config = ConfigDict(extra="forbid")
    page: int = Field(ge=1, le=10)
    excerpt: str = Field(min_length=1, max_length=512)


class Extraction(BaseModel):
    model_config = ConfigDict(extra="forbid")
    fields: dict[str, str | None]
    evidence: dict[str, Evidence]
    warnings: list[str]


class Provider(Protocol):
    def extract(self, pages: list[str]) -> dict: ...


class MockProvider:
    """Recognize only the printed labels in the fictional English fixtures."""

    def extract(self, pages: list[str]) -> dict:
        fields = dict.fromkeys(LABELS.values())
        evidence, warnings = {}, []
        for number, text in enumerate(pages, 1):
            for line in text.splitlines():
                label, sep, value = line.partition(":")
                if label.strip().lower() in {
                    "discount",
                    "credit note",
                    "reverse charge",
                }:
                    warnings.append(f"Unsupported adjustment: {label.strip()}")
                key = LABELS.get(label.strip())
                if not sep or key is None or not value.strip():
                    continue
                if fields[key] is not None:
                    warnings.append(f"Repeated label for {key}")
                    continue
                fields[key] = value.strip()
                evidence[key] = {"page": number, "excerpt": line}
        return {"fields": fields, "evidence": evidence, "warnings": warnings}


def page_text(path: Path) -> list[str]:
    if path.stat().st_size > 5_000_000:
        raise DocumentError("INVALID_DOCUMENT")
    with pymupdf.open(path) as doc:
        if not doc.is_pdf or doc.needs_pass or not 1 <= len(doc) <= 10:
            raise DocumentError("INVALID_DOCUMENT")
        pages = [page.get_text() for page in doc]
    if not any(p.strip() for p in pages):
        raise DocumentError("OCR_REQUIRED")
    return pages


def extract(path: Path, provider: Provider | None = None) -> Extraction:
    try:
        pages = page_text(path)
    except DocumentError as error:
        if str(error) != "OCR_REQUIRED":
            raise
        return Extraction(
            fields=dict.fromkeys(LABELS.values()),
            evidence={},
            warnings=["OCR_REQUIRED"],
        )
    if provider is None:
        if os.getenv("AI_MODE", "mock") != "mock":
            raise DocumentError("PROVIDER_ERROR")
        provider = MockProvider()
    for _ in range(2):
        try:
            result = Extraction.model_validate(provider.extract(pages))
            if set(result.fields) != set(LABELS.values()) or any(
                v is not None and len(v) > 256 for v in result.fields.values()
            ):
                raise ValueError("Invalid fields")
            for key in tuple(result.evidence):
                e = result.evidence[key]
                if (
                    key not in result.fields
                    or result.fields[key] is None
                    or result.fields[key] not in e.excerpt
                    or e.page > len(pages)
                    or e.excerpt not in pages[e.page - 1]
                ):
                    del result.evidence[key]
            for key, value in result.fields.items():
                if value is not None and key not in result.evidence:
                    result.warnings.append(f"Unverified evidence for {key}")
            return result
        except (ValidationError, ValueError):
            continue
    raise DocumentError("SCHEMA_ERROR")


def fixture(sample_id: str) -> Path:
    if not re.fullmatch(r"[a-z][a-z0-9-]{0,63}", sample_id):
        raise DocumentError("INVALID_DOCUMENT")
    path = Path(os.getenv("FIXTURE_DIR", "/fixtures/invoices")) / f"{sample_id}.pdf"
    if not path.is_file():
        raise DocumentError("INVALID_DOCUMENT")
    return path
