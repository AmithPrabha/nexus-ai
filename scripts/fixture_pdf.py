"""Write a one-page fictional PDF from labeled lines (ASCII demo fixtures)."""

from pathlib import Path

LABELS = {
    "supplier_name": "Supplier",
    "supplier_trn": "Supplier TRN",
    "invoice_number": "Invoice",
    "invoice_date": "Date",
    "currency": "Currency",
    "subtotal": "Subtotal",
    "vat_rate": "VAT rate",
    "vat_amount": "VAT amount",
    "total": "Total",
    "tax_treatment": "Treatment",
}


def write_pdf(path: Path, fields: dict):
    def literal(line):
        return line.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")

    lines = ["NEXUSAI  /  FICTIONAL INVOICE", "Sample for simulated review", ""]
    lines += [
        f"{label}: {fields[key]}"
        for key, label in LABELS.items()
        if fields.get(key) is not None
    ]
    stream = (
        "BT /F 15 Tf 52 774 Td 25 TL "
        + " T* ".join(f"({literal(s)}) Tj" for s in lines)
        + " ET"
    ).encode("ascii")
    objects = [
        b"<</Type/Catalog/Pages 2 0 R>>",
        b"<</Type/Pages/Kids[3 0 R]/Count 1>>",
        b"<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Resources<</Font<</F 4 0 R>>>>/Contents 5 0 R>>",
        b"<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>",
        f"<</Length {len(stream)}>>\nstream\n".encode() + stream + b"\nendstream",
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for i, obj in enumerate(objects, 1):
        offsets.append(len(out))
        out.extend(f"{i} 0 obj\n".encode() + obj + b"\nendobj\n")
    start = len(out)
    out.extend(f"xref\n0 6\n0000000000 65535 f \n".encode())
    for offset in offsets[1:]:
        out.extend(f"{offset:010d} 00000 n \n".encode())
    out.extend(f"trailer\n<</Size 6/Root 1 0 R>>\nstartxref\n{start}\n%%EOF\n".encode())
    path.write_bytes(out)


if __name__ == "__main__":
    # Reproduce the original five allowlisted PDFs from their fictional source data.
    import argparse

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path, help="Directory for generated PDF samples")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    base = dict(
        supplier_name="Saffron Office Supply LLC",
        supplier_trn="100123456789012",
        invoice_date="2026-01-15",
        currency="AED",
        subtotal="480.00",
        vat_rate="0.05",
        vat_amount="24.00",
        total="504.00",
        tax_treatment="standard",
    )
    cases = [
        ("office-supplies", {}),
        ("design-services", {"vat_amount": "20.00", "total": "500.00"}),
        ("courier", {"supplier_trn": None}),
        (
            "equipment",
            {"subtotal": "10000.00", "vat_amount": "500.00", "total": "10500.00"},
        ),
        ("maintenance", {"supplier_trn": "TRN123"}),
    ]
    for number, (name, overrides) in enumerate(cases, 1):
        write_pdf(
            args.output / (name + ".pdf"),
            base | {"invoice_number": f"NX-{number:04d}"} | overrides,
        )
