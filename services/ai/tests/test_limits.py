import pymupdf
import pytest

from app.extract import DocumentError, page_text


@pytest.mark.parametrize(
    ("pages", "code"), [(1, "OCR_REQUIRED"), (11, "INVALID_DOCUMENT")]
)
def test_pdf_limits(tmp_path, pages, code):
    path = tmp_path / "fictional.pdf"
    with pymupdf.open() as doc:
        for _ in range(pages):
            doc.new_page()
        doc.save(path)
    with pytest.raises(DocumentError, match=code):
        page_text(path)
