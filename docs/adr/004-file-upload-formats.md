# ADR 004: File Upload Formats — Images, CSV, XLSX, OFX, TXT

**Status:** Accepted  
**Date:** 2025-04-21  
**Deciders:** Project owner (Gabriel) + AI assistant

---

## Context

Users need to add transactions to Actual Budget from various sources. The primary use case (per project owner) is screenshots from credit card statements. Other sources include receipts, bank exports, and manual files.

## Constraints

- Must handle the project owner's primary workflow (credit card statement screenshots)
- Must support common bank export formats
- Must validate and sanitize uploads
- Must extract structured data from each format
- Should provide a unified preview and confirmation flow

## Alternatives Considered

### Alternative A: Images Only

**Description:** Support only image uploads (receipts, screenshots).

**Pros:**
- Simplest implementation — always send to LLM vision
- Unified processing path

**Cons:**
- LLM vision costs more per transaction for bulk data
- Wasteful for structured data like CSV
- Users with bank exports are left out

**Rejected because:** Doesn't cover bank export formats (CSV, OFX). The project owner specifically mentioned CSV/XLSX/TXT/OFX support.

### Alternative B: Structured Files Only

**Description:** Support only structured data files (CSV, XLSX, OFX, TXT).

**Pros:**
- Cheapest — no LLM vision costs
- Deterministic parsing
- Fast processing

**Cons:**
- Doesn't handle receipts or screenshots
- The project owner's primary workflow (credit card statements) is screenshot-based

**Rejected because:** Excludes the main use case. Credit card statement screenshots are the primary input method.

### Alternative C: All Formats with Unified Flow

**Description:** Support images (JPG, PNG), spreadsheets (CSV, XLSX), bank formats (OFX), and text (TXT). Route each to the appropriate parser, then unify into a common preview/confirmation flow.

**Pros:**
- Covers all use cases
- Optimized processing per format (vision for images, Pandas for CSV/XLSX, OFX parser for OFX)
- Single user experience: upload → preview → confirm

**Cons:**
- More code paths to maintain
- Need to handle format-specific edge cases

**Accepted because:** Best user experience. The complexity is manageable — each parser is isolated and well-defined.

## Decision

Support **images (JPG, PNG), CSV, XLSX, OFX, and TXT** with a unified upload → preview → confirm flow.

## Tradeoffs

- **Coverage vs complexity:** We accept multiple parser implementations for maximum user flexibility.
- **Cost vs convenience:** Images go to LLM vision (more expensive), structured files are parsed locally (cheaper). Users can choose their preferred method.

## Implementation Approach

```
Upload Router
    │
    ├── Image (JPG/PNG)
    │       └── LLM Vision API → JSON transactions
    │
    ├── Spreadsheet (CSV/XLSX)
    │       └── Pandas → DataFrame → JSON transactions
    │
    ├── Bank Format (OFX)
    │       └── OFX parser (ofxparse or similar) → JSON transactions
    │
    └── Text (TXT)
            └── LLM text parsing → JSON transactions
    │
    ▼
Unified Preview Component
    │
    ├── Confirm All → Actual Budget API
    ├── Edit Individual → re-parse with edits
    └── Reject → discard
```

### Parser Details

| Format | Library | Approach |
|--------|---------|----------|
| JPG/PNG | LLM Vision (OpenAI/Anthropic) | Base64 encode, send to vision model with extraction prompt |
| CSV | Pandas | Read → map columns → extract transactions |
| XLSX | Pandas + openpyxl | Read → map columns → extract transactions |
| OFX | ofxparse | Parse XML → extract transactions |
| TXT | LLM text | Send raw text with extraction prompt |

### Validation

- File size limit: 10MB
- MIME type validation (not just extension)
- Virus scanning: out of scope for MVP (user's self-hosted environment)
- Storage: outside web root, organized by upload date

## Consequences

- **Positive:** Users can upload any common format. Primary workflow (screenshots) is supported natively.
- **Negative:** More testing needed across file format variations. We may need to iterate on column mapping for CSV/XLSX from different banks.
