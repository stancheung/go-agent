---
name: doc-processing
description: Extract, edit, and process PDFs and documents — OCR/text extraction, text editing, page manipulation
pinned: false
category: productivity
---

# Document Processing

Unified skill for working with PDFs and scanned documents: extract text (including OCR), edit content, split/merge pages.

## §1 Text Extraction (from ocr-and-documents)

### Step 1: Try web_extract First
For remote URLs, always use `web_extract` — handles PDF-to-markdown via Firecrawl with no local deps:
```
web_extract(urls=["https://example.com/report.pdf"])
```

Only use local extraction when: file is local, web_extract fails, or batch processing needed.

### Step 2: Choose Local Extractor

| Feature | pymupdf (~25MB) | marker-pdf (~3-5GB) |
|---------|-----------------|---------------------|
| Text-based PDF | ✅ | ✅ |
| Scanned PDF (OCR) | ❌ | ✅ (90+ languages) |
| Tables | ✅ (basic) | ✅ (high accuracy) |
| Equations / LaTeX | ❌ | ✅ |
| Speed | Instant | ~1-14s/page |

**Decision:** Use pymupdf unless you need OCR, equations, or complex layout analysis.

#### pdf skill scripts (preferred — deps already installed)
```bash
python skills/pdf/scripts/pdf_read.py document.pdf --text    # Per-page text (JSON)
python skills/pdf/scripts/pdf_read.py document.pdf --meta    # Metadata + scanned-page check
```

#### pymupdf (needs `pip install pymupdf pymupdf4llm`)
```bash
python skills/pdf/scripts/extract_pymupdf.py document.pdf              # Plain text
python skills/pdf/scripts/extract_pymupdf.py document.pdf --markdown    # Markdown
python skills/pdf/scripts/extract_pymupdf.py document.pdf --tables      # Tables
```

#### marker-pdf OCR (needs `pip install marker-pdf`, ~2.5GB models)
```bash
python skills/pdf/scripts/extract_marker.py document.pdf                # Markdown
python skills/pdf/scripts/extract_marker.py scanned.pdf                 # OCR mode
```

### Split, Merge & Search (pymupdf)
```python
# Split: pages 1-5 to new PDF
import pymupdf
doc = pymupdf.open("report.pdf")
new = pymupdf.open()
for i in range(5): new.insert_pdf(doc, from_page=i, to_page=i)
new.save("pages_1-5.pdf")

# Merge multiple PDFs
result = pymupdf.open()
for path in ["a.pdf", "b.pdf", "c.pdf"]: result.insert_pdf(pymupdf.open(path))
result.save("merged.pdf")
```

## §2 Text Editing (from nano-pdf)

Edit PDF content using natural-language instructions via the nano-pdf CLI:

```bash
# Install
uv pip install nano-pdf

# Edit text on a page
nano-pdf edit deck.pdf 1 "Change the title to 'Q3 Results' and fix the typo in the subtitle"
nano-pdf edit report.pdf 3 "Update the date from January to February 2026"
nano-pdf edit contract.pdf 2 "Change the client name from 'Acme Corp' to 'Acme Industries'"
```

**Notes:**
- Page numbers may be 0-based or 1-based — retry with ±1 if wrong page
- Verify output PDF after editing
- Requires an API key (check `nano-pdf --help` for config)
- Best for text changes; complex layout modifications need different approach

## Support Files
- The `pdf` skill (`skills/pdf/`) has richer tooling: `pdf_read.py`, `pdf_split.py`, `pdf_merge.py`, etc. — see its SKILL.md.
