# FinWise Reactive Stream — System Architecture

**Status:** Draft  
**Date:** 2026-04-21  
**Based on:** `docs/designs/reactive-stream.md` (CEO Plan), `docs/specs/2026-04-21-finwise-phase1-design.md`, ADRs 001–005  

---

## 1. Overview

FinWise Reactive Stream turns FinWise from a manual capture tool into an autonomous bookkeeper. When a user uploads a credit card statement screenshot, bulk CSV, or receipt, the system initiates a **Server-Sent Events (SSE) stream** that delivers extracted transactions in real time. The user reviews, edits inline, and confirms — all within a single chat-like interface.

### 1.1 Key Principles

- **Self-hosted first:** Single `docker compose up` deployment.
- **Zero-duplication:** FinWise stores only pending/outbox data; all reporting hits Actual Budget (AB) directly.
- **Resilient streaming:** SSE auto-reconnects with `Last-Event-ID` resume. Server state is the source of truth.
- **Provider-agnostic LLM:** Custom thin wrapper supports OpenAI, Anthropic, Ollama, and OpenAI-compatible APIs (ADR-002).
- **Learning system:** Frequency-based payee mappings improve extraction accuracy over time.

---

## 2. Scope

### 2.1 In Scope

| Feature | Description |
|---------|-------------|
| **Reactive Stream Chat** | SSE-based real-time transaction extraction with narration |
| **Non-Streaming Fallback** | Batch mode for >30 transactions, failed EventSource, or stalled streams |
| **Bulk File Upload** | CSV, XLSX, OFX, TXT parsing with unified preview/confirm flow |
| **OCR Pre-processing Fallback** | Tesseract/EasyOCR pipeline if LLM vision accuracy is insufficient |
| **Confidence Calibration Dashboard** | Histogram of confidence vs. correction rate; suggested auto-sync threshold |
| **Micro-delights** | Enter-to-send, Shift+Enter newline, drag-and-drop, dark mode, subtle sync sound |

### 2.2 Out of Scope (Deferred)

- Ambient reconciliation (Phase 3+)
- Multi-persona scaffolding beyond Bookkeeper (Phase 2+)
- Browser extension for Amazon/Uber scraping (Phase 4)
- Standalone budget engine (Phase 5)

---

## 3. Quality Attributes

| Attribute | Target | Measurement |
|-----------|--------|-------------|
| **Performance** | First transaction event within **10 seconds** for <30 transactions | SSE p95 latency |
| **Accuracy** | ≥**85%** correct field extraction on benchmark dataset | Automated benchmark |
| **OCR Trigger** | Enable OCR fallback when LLM vision accuracy drops below **80%** | Correction rate monitoring |
| **Reliability** | High-confidence (≥0.9) correction rate <10%; overall <30% | Dashboard metrics |
| **Availability** | SSE auto-reconnect; zero data loss on disconnect | Integration tests |
| **Scalability** | Single-user self-hosted; SQLite default, PostgreSQL optional | Load test |
| **Security** | API keys encrypted at rest; file uploads validated | Audit / penetration test |

---

## 4. System Context (C4 Level 1)

```mermaid
graph TB
    User["🧑 User<br/>(Browser: HTMX + EventSource)"]
    FinWise["📦 FinWise<br/>(Self-Hosted)"]
    AB["💰 Actual Budget<br/>(User's Instance)"]
    LLM["🤖 LLM Provider<br/>(OpenAI / Anthropic / Ollama)"]
    OCR["🔍 OCR Service<br/>(Tesseract / EasyOCR — conditional)"]

    User -->|"Uploads statements,<br/>reviews transactions"| FinWise
    FinWise -->|"Sync confirmed transactions"| AB
    FinWise -->|"Extract from images/text"| LLM
    FinWise -->|"Fallback text extraction"| OCR
```

---

## 5. Container Architecture (C4 Level 2)

```mermaid
graph TB
    subgraph "User Browser"
        UI["HTMX + DaisyUI/Tailwind<br/>Jinja2 Templates<br/>Raw EventSource Client"]
    end

    subgraph "FinWise Server (Docker)"
        FastAPI["FastAPI Application<br/>(Python 3.11+)"]
        SQLite[("SQLite<br/>finwise.db")]
        FS["Local Filesystem<br/>./uploads"]
    end

    subgraph "External Systems"
        AB_API["Actual Budget API"]
        LLM_API["LLM Provider API"]
        OCR_LIB["OCR Library<br/>(in-process)"]
    end

    UI -->|"HTTP / SSE"| FastAPI
    FastAPI -->|"SQLAlchemy / Alembic"| SQLite
    FastAPI -->|"Read/Write"| FS
    FastAPI -->|"HTTPS"| AB_API
    FastAPI -->|"HTTPS"| LLM_API
    FastAPI -->|"Local call"| OCR_LIB
```

---

## 6. Component Architecture (C4 Level 3)

```mermaid
graph TB
    subgraph "Routers (FastAPI)"
        R_CHAT["/chat/*<br/>Chat Router"]
        R_UPLOAD["/chat/upload<br/>Upload Router"]
        R_TXN["/transactions/*<br/>Transaction Router"]
        R_SETTINGS["/settings<br/>Settings Router"]
    end

    subgraph "Service Layer"
        S_BOOK["Bookkeeper Service"]
        S_UPLOAD["Upload Orchestrator Service"]
        S_AB["Actual Budget Client"]
        S_LLM["LLM Client<br/>(Provider-Agnostic Wrapper)"]
        S_OCR["OCR Service"]
        S_LEARN["Learning Service"]
    end

    subgraph "Data Layer"
        DB_SETTINGS["Settings Model<br/>(SQLAlchemy)"]
        DB_MSG["Message Model<br/>(SQLAlchemy)"]
        DB_TXN["PendingTransaction Model<br/>(SQLAlchemy)"]
        DB_MAP["PayeeMapping Model<br/>(SQLAlchemy)"]
    end

    subgraph "Background Tasks"
        T_STREAM["Stream Processor<br/>(async generator)"]
        T_SYNC["Sync Worker"]
    end

    R_CHAT --> S_BOOK
    R_UPLOAD --> S_UPLOAD
    R_TXN --> S_UPLOAD
    R_SETTINGS --> S_BOOK

    S_BOOK --> S_LLM
    S_BOOK --> S_LEARN
    S_UPLOAD --> S_LLM
    S_UPLOAD --> S_OCR
    S_UPLOAD --> T_STREAM
    S_UPLOAD --> T_SYNC

    S_BOOK --> DB_SETTINGS
    S_BOOK --> DB_MSG
    S_UPLOAD --> DB_TXN
    S_LEARN --> DB_MAP
    T_SYNC --> S_AB
    T_SYNC --> DB_TXN
```

### 6.1 Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| **Chat Router** | Handles chat messages, initiates extraction jobs, serves SSE stream endpoint. |
| **Upload Router** | Accepts file uploads, validates MIME/type, delegates to Upload Orchestrator. |
| **Transaction Router** | Inline edit, confirm single/bulk, reject. Returns HTMX partials. |
| **Settings Router** | CRUD for app configuration, LLM credentials, AB connection. |
| **Bookkeeper Service** | Orchestrates LLM calls for natural language and image inputs. Manages chat context. Queries SQLAlchemy models directly. |
| **Upload Orchestrator** | Routes files to correct parser (vision, pandas, OFX, text). Manages extraction jobs. Queries SQLAlchemy models directly. |
| **LLM Client** | Thin wrapper over OpenAI, Anthropic, Ollama. Supports `chat`, `chat_with_image`, JSON schema mode. |
| **Actual Budget Client** | Wrapper around AB API. Handles authentication, rate limiting, retries, and error propagation. |
| **OCR Service** | Conditional fallback. Extracts raw text from images using Tesseract or EasyOCR. |
| **Learning Service** | Normalizes payees, upserts `payee_mappings`, injects top-N mappings into system prompts. Queries SQLAlchemy models directly. |
| **Stream Processor** | Async generator that yields SSE events from DB job state. Handles resume logic. |
| **Sync Worker** | Background task that pushes confirmed transactions to AB. |

**Note:** There is no Repository layer. Services query SQLAlchemy models directly to reduce abstraction overhead for this single-user application.

---

## 7. Data Architecture

### 7.1 Database Selection

- **Default:** SQLite (`sqlite:///data/finwise.db`)
- **Optional:** PostgreSQL via `DATABASE_URL`
- **ORM:** SQLAlchemy 2.0 with Alembic migrations
- **Encryption:** Sensitive fields (API keys, passwords) encrypted at the application layer using Fernet (`FINWISE_SECRET_KEY`).

### 7.2 Entity Relationship Diagram

```mermaid
erDiagram
    SETTINGS ||--o{ CHAT_SESSIONS : "has"
    CHAT_SESSIONS ||--o{ MESSAGES : "contains"
    MESSAGES ||--o{ PENDING_TRANSACTIONS : "generates"
    PENDING_TRANSACTIONS ||--o{ SYNC_LOG : "produces"
    PENDING_TRANSACTIONS ||--o{ PAYEE_MAPPINGS : "learns from"

    SETTINGS {
        int id PK
        string actual_budget_url
        string actual_budget_password "encrypted"
        string llm_provider
        string llm_api_key "encrypted"
        string llm_model
        decimal auto_sync_threshold "default 1.0"
        boolean use_streaming
        string default_currency
        datetime created_at
        datetime updated_at
    }

    CHAT_SESSIONS {
        int id PK
        string persona "default 'bookkeeper'"
        int active_job_id "nullable; tracks current extraction job"
        datetime created_at
    }

    MESSAGES {
        int id PK
        int session_id FK
        string role "user|assistant|system"
        text content
        json attachments
        datetime created_at
    }

    PENDING_TRANSACTIONS {
        int id PK
        int message_id FK
        date txn_date
        string payee
        decimal amount
        string category
        string notes
        decimal confidence "0.0-1.0"
        string status "pending|synced|needs_review|rejected"
        string source "nlp|receipt|screenshot|csv|ofx|txt"
        json raw_data
        string dedup_hash "SHA256(date+payee+amount)"
        string actual_budget_id "nullable; foreign reference"
        string sync_error
        datetime created_at
        datetime updated_at
    }

    SYNC_LOG {
        int id PK
        int pending_transaction_id FK
        string actual_budget_transaction_id
        string sync_status "success|failed"
        text error_message
        datetime synced_at
    }

    PAYEE_MAPPINGS {
        int id PK
        string payee "normalized, unique"
        string category
        int frequency "default 1"
        datetime last_used
        datetime created_at
        datetime updated_at
    }
```

### 7.3 Key Data Decisions

| Decision | Rationale |
|----------|-----------|
| **`pending_transactions.actual_budget_id`** | **Foreign reference pointer only.** Stores the UUID/string ID returned by Actual Budget after sync. This is not duplicated transactional data; it is a link back to the source of truth in AB. |
| **`dedup_hash`** | SHA-256 of normalized `date + payee + amount + upload_id`. Prevents duplicate insertions from re-uploaded statements or retries. Includes `upload_id` to handle recurring purchases correctly. |
| **`payee_mappings.payee`** | Normalized (lowercase, stripped punctuation) to ensure stable matching. |
| **Archiving strategy** | Transactions with `status='synced'` and `updated_at < NOW() - INTERVAL '90 days'` are soft-deleted or moved to an `archived_transactions` table on a nightly cron/job. |
| **Cold start seeding** | On first run (or when `payee_mappings` is empty), the Learning Service queries AB's existing transaction history to seed the mapping table. |

### 7.4 Database Indexes

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_pending_job_created` | `pending_transactions(job_id, created_at)` | SSE resume logic - efficiently query missed events after reconnect |
| `idx_pending_confidence_status` | `pending_transactions(confidence, status, created_at)` | Calibration dashboard queries and auto-sync threshold filtering |
| `idx_pending_dedup` | `pending_transactions(dedup_hash)` | Fast deduplication checks during ingestion |
| `idx_payee_frequency` | `payee_mappings(frequency DESC)` | Top-N payee lookup for prompt injection |

---

## 8. API & SSE Protocol

### 8.1 REST Endpoints

| Route | Method | Description | Response |
|-------|--------|-------------|----------|
| `POST /chat/upload` | multipart/form-data | Initiates extraction job. Returns `{ job_id, stream_url }`. | JSON |
| `GET /chat/stream/{job_id}` | SSE | Real-time stream of extraction events. Supports `Last-Event-ID` header for resume. | `text/event-stream` |
| `POST /transactions/{id}/edit` | HTMX (form) | Inline edit of a pending transaction. Returns updated row HTML. | HTML partial |
| `POST /transactions/{id}/confirm` | HTMX | Confirm single transaction → sync to AB. Returns updated status badge. | HTML partial |
| `POST /transactions/bulk-confirm` | HTMX | Confirm all high-confidence (`confidence >= auto_sync_threshold`) pending transactions. | HTML partial |
| `GET /settings` | — | Settings page. | HTML page |
| `POST /settings` | Form | Save settings. | Redirect |

### 8.2 SSE Event Specification

**Connection:** Raw `EventSource` (no library needed). Client sends `Last-Event-ID` header on reconnect.

**Heartbeat:** Server emits `heartbeat` every 5 seconds. Client treats 10s without any event as a stall → triggers fallback mode.

| Event Type | Payload (JSON) | Description |
|------------|----------------|-------------|
| `narration` | `{ "text": "Found 23 transactions..." }` | Bookkeeper narration text. Emitted as a separate SSE event by the server, derived from the `narration` field in the LLM JSON response. |
| `transaction` | `{ "id": 123, "date": "2024-01-15", "payee": "Starbucks", "amount": -12.50, "category": "Food", "confidence": 0.95, "status": "pending" }` | Single extracted transaction. |
| `sync_status` | `{ "synced_count": 5, "pending_review_count": 2, "error_count": 0 }` | Batch progress update after confirmations. |
| `complete` | `{ "total": 23, "auto_synced": 18, "needs_review": 5 }` | Final summary when extraction is done. |
| `error` | `{ "stage": "llm_extract", "message": "...", "recoverable": true }` | Structured error. If `recoverable`, client shows retry button. |
| `heartbeat` | `{}` | Keep-alive. Empty payload. |

### 8.3 SSE Resume Logic

1. Client connects to `GET /chat/stream/{job_id}`.
2. On disconnect, client stores last received `id` (event ID).
3. On reconnect, client sends `Last-Event-ID: <id>`.
4. Server queries `pending_transactions` for that `job_id` where `id > last_event_id` (or `created_at > last_timestamp`), replays missed transactions, then continues live stream.
5. **Source of truth:** The database (`pending_transactions` table) is the canonical state. The SSE stream is a read-only projection.

### 8.4 Fallback Triggers

Non-streaming mode activates when:
- Statement contains >30 transactions (configurable via `settings.use_streaming_threshold`).
- Client `EventSource` fails to connect (e.g., corporate proxy blocks SSE).
- Stream stalls for >10 seconds without heartbeat.

In fallback mode:
1. `POST /chat/upload` → server blocks until processing complete.
2. Returns full HTML batch preview via HTMX swap.
3. Review and editing still happen inline in chat.

---

## 9. File Upload & Parsing Pipeline

```mermaid
flowchart TD
    A["User Uploads File"] --> B{"Detect Type<br/>(MIME + Extension)"}
    B -->|Image: JPG/PNG| C["LLM Vision API<br/>Base64 encode"]
    B -->|Spreadsheet: CSV/XLSX| D["Pandas Parser<br/>Column mapping"]
    B -->|Bank Format: OFX| E["OFX Parser<br/>(ofxparse)"]
    B -->|Text: TXT| F["LLM Text Parsing"]
    B -->|Unsupported| G["Reject: 400 Bad Request"]

    C --> H["Transaction Structs"]
    D --> H
    E --> H
    F --> H

    H --> I["Deduplication Check<br/>SHA256(date+payee+amount)"]
    I -->|Exists| J["Skip / Update if changed"]
    I -->|New| K["Insert into<br/>pending_transactions"]

    K --> L["Start SSE Stream<br/>or Return Batch HTML"]
```

### 9.1 Parser Details

| Format | Library | Approach |
|--------|---------|----------|
| **JPG/PNG** | LLM Vision (OpenAI GPT-4o) | Base64 encode image, send with structured JSON schema prompt. |
| **CSV** | Pandas | `read_csv` → map columns (date, payee, amount) → normalize. |
| **XLSX** | Pandas + openpyxl | `read_excel` → map columns → normalize. |
| **OFX** | ofxparse | Parse XML → extract `STMTTRN` entries. |
| **TXT** | LLM text | Send raw text with extraction prompt (same JSON schema). |

### 9.2 Validation Rules

- Max file size: 10MB.
- MIME type must match extension.
- Images: minimum resolution check (to prevent unreadable thumbnails).
- CSV/XLSX: header row required; reject if zero rows parsed.

---

## 10. LLM Prompt Strategy

### 10.1 Primary Provider

**OpenAI GPT-4o** with native JSON schema response format (`response_format={"type": "json_schema", ...}`).

### 10.2 System Prompt Structure

```markdown
You are the Bookkeeper. Extract all visible transactions from the input.

Available categories: {{ categories }}
Known user preferences (top-10 payees): {{ payee_mappings }}

Return JSON matching this schema:
{
  "narration": "friendly summary of what was found",
  "transactions": [
    {
      "date": "YYYY-MM-DD",
      "payee": "string",
      "amount": -12.50,
      "category": "string",
      "notes": "string",
      "confidence": 0.95
    }
  ]
}
```

### 10.3 Extraction Stages

**MVP:** Single-stage extraction.
- Prompt asks for all visible transactions in one call.
- Returns `narration` + `transactions[]` array with per-transaction `confidence`.
- No separate narration LLM call.

**Chunking:** For statements with >50 transactions:
- Split image into logical segments or crop regions (if possible).
- Otherwise, prompt asks for first 25, then second 25, etc.
- Server aggregates chunks into a single job stream.

### 10.4 Narration Delivery Clarification

- The LLM returns narration text in the `narration` field of the JSON response.
- The **server** extracts this field and emits it as a separate SSE event of type `narration` before emitting the `transaction` events.
- This decouples the LLM response structure from the wire protocol.
- Client receives narration as a standalone event, not embedded in transaction payloads.

---

## 11. OCR Fallback Pipeline

### 11.1 Trigger Condition

- **Decision gate:** Run 10–20 real statements through GPT-4o vision.
- If correction rate exceeds 20% (i.e., accuracy <80%), enable OCR fallback.
- **Target accuracy:** 85%. **OCR trigger:** <80%.

### 11.2 Fallback Flow

```mermaid
flowchart LR
    A["Image Upload"] --> B{"OCR Enabled?"}
    B -->|No| C["LLM Vision Direct"]
    B -->|Yes| D["OCR Extracts Raw Text"]
    D --> E["Send Text to LLM<br/>(non-vision prompt)"]
    E --> F["Structured JSON<br/>Transactions"]
    C --> F
```

### 11.3 Technology

- **Primary:** Tesseract (local, no network dependency).
- **Alternative:** EasyOCR (better for handwritten text, heavier dependency).
- **Selection:** Start with Tesseract; evaluate EasyOCR if accuracy is insufficient.

---

## 12. Learning & Confidence Mechanism

### 12.1 Frequency-Based Rule Learning

1. **Normalize:** `payee.lower().strip().translate(remove_punctuation)`.
2. **Upsert:** `INSERT OR REPLACE INTO payee_mappings (payee, category, frequency, last_used)`.
3. **Increment:** `frequency = frequency + 1`.
4. **Injection:** On next extraction, select **top-10** mappings by `frequency` (i.e., the 10 most frequently occurring payees) and inject into the LLM system prompt as "User's known preferences."

### 12.2 Cold Start

- On first run (when `payee_mappings` table is empty), query Actual Budget's existing transaction history.
- Normalize payees from AB history and seed the mapping table.
- This ensures the LLM has context even before the user has confirmed any FinWise transactions.

### 12.3 Confidence Calibration Dashboard

A dedicated page at `/dashboard/calibration` showing:
- **Histogram:** LLM confidence score buckets vs. actual correction rate over time.
- **Suggestion:** Recommended `auto_sync_threshold` based on historical accuracy (e.g., "Set to 0.92 to auto-sync 80% of transactions with <5% error").
- **Per-payee accuracy:** Table of payees with average confidence and actual correction rate.

### 12.4 Auto-Sync Logic

```python
if transaction.confidence >= settings.auto_sync_threshold:
    status = "pending"  # Still inserted, but flagged for bulk-confirm
    # bulk-confirm endpoint will auto-sync these
else:
    status = "needs_review"
```

---

## 13. Reliability & Resilience

### 13.1 Concurrent Upload Handling

- **Max 1 active job per user/session.**
- If a new upload arrives while a job is active:
  - **Reject** with `409 Conflict` and message: "A job is already running. Please wait or cancel it."
- Track active job via `chat_sessions.active_job_id` column (nullable FK to extraction job).
- On job completion (success/error/cancel), clear `active_job_id` to allow new uploads.

### 13.2 Deduplication

- Before inserting into `pending_transactions`, compute `dedup_hash = SHA256(date + normalized_payee + amount + upload_id)`.
- **Why include `upload_id`:** Recurring purchases (e.g., "Starbucks $5 on the 15th") would collide without the upload source identifier.
- If hash exists for the current job, skip insertion or update existing record if data has changed.
- Prevents duplicates from re-uploaded statements or retries.

### 13.3 Rate Limiting for Actual Budget API

- **Max 10 sync calls per minute** to AB.
- Exponential backoff with jitter on `429 Too Many Requests` or connection errors.
- Sync Worker uses an async semaphore or token bucket.
- Failed syncs populate `sync_error` and retry up to 3 times before surfacing to user.

### 13.4 Archiving Strategy

- **Nightly job** (or async background task) runs `DELETE` / `INSERT INTO archived_transactions` for records where:
  - `status = 'synced'`
  - `updated_at < NOW() - INTERVAL '90 days'`
- **Soft-delete approach:** Add `deleted_at` timestamp column instead of hard delete for audit trail.
- Keeps `pending_transactions` table small and fast for active review queries.

### 13.5 HTMX Swap Strategy

| Interaction | HTMX Attributes | Target | Swap |
|-------------|-----------------|--------|------|
| **Inline transaction edit** | `hx-post="/transactions/{id}/edit"` | `#tx-{id}` | `innerHTML` |
| **Confirm single transaction** | `hx-post="/transactions/{id}/confirm"` | `#tx-{id}-status` | `outerHTML` |
| **Bulk confirm all** | `hx-post="/transactions/bulk-confirm"` | `#transaction-list` | `innerHTML` |
| **New chat message** | `hx-post="/chat/send"` | `#chat-messages` | `beforeend` |
| **File upload progress** | `hx-post="/chat/upload"` + SSE | `#upload-progress` | `innerHTML` |
| **Settings save** | `hx-post="/settings"` | `#settings-form` | `none` (redirect on success) |
| **Toast notifications** | `hx-swap-oob="true"` | `#toast-container` | `afterbegin` |

### 13.6 Error Handling Matrix

| Scenario | Server Behavior | Client Behavior |
|----------|----------------|-----------------|
| LLM API fails / invalid key | Stream emits `error` event. Pending transactions stay `pending`. | Show error in chat. Provide "Retry" button. |
| LLM returns malformed JSON | Stream emits `error` with `stage: "llm_parse"`. Log raw response. | Show raw response in collapsible block for user assistance. |
| AB API fails mid-sync | Synced transactions remain synced. Unsynced stay `pending` with `sync_error`. | Highlight failed transactions with retry button. |
| SSE connection drops | Server resumes from `Last-Event-ID` via DB state. | Auto-reconnect with `Last-Event-ID`. |
| File upload invalid | Immediate `400 Bad Request` with list of supported formats. | Show validation message. |
| Transaction validation fails | Stream pauses. Offending transaction highlighted. | Inline edit via HTMX swap (`hx-target="#tx-{id}"`, `hx-swap="innerHTML"`). |
| User rejects transaction | Mark `status = 'rejected'`. Do not sync to AB. | Remove from preview or grey out. |

### 13.7 Rate Limiting for Actual Budget API

- **Max 10 sync calls per minute** to AB API.
- **Implementation:** Token bucket algorithm with async semaphore.
- **Backoff strategy:** Exponential backoff with jitter on `429 Too Many Requests` or connection errors.
  - Initial delay: 1s
  - Max delay: 30s
  - Jitter: ±20% randomization
- **Max retries:** 3 attempts before surfacing error to user.
- **Failed sync handling:** Populate `sync_error` column, mark transaction as `needs_review`, allow manual retry.

---

## 14. Security

| Concern | Mitigation |
|---------|------------|
| **API key storage** | Encrypt at rest with Fernet (`FINWISE_SECRET_KEY`). Keys never logged. |
| **Actual Budget password** | Same Fernet encryption. Stored only in `settings` table. |
| **File uploads** | Validate MIME type and extension. Store outside web root (`./uploads`). Max 10MB. |
| **Session / auth** | Single-user app. Optional basic auth via `FINWISE_PASSWORD` env var. |
| **HTTPS** | Required for production. Document reverse proxy setup (Caddy/Nginx). |
| **LLM data privacy** | User controls provider. Local Ollama supported for full privacy. |
| **Input sanitization** | All user inputs (edit forms) validated with Pydantic schemas. HTMX prevents most XSS; Jinja2 auto-escapes. |

---

## 15. Deployment

### 15.1 Docker Compose

```yaml
version: '3.8'
services:
  finwise:
    image: finwise:latest
    build: .
    ports:
      - "8000:8000"
    env_file:
      - .env
    volumes:
      - ./data:/app/data
      - ./uploads:/app/uploads
    restart: unless-stopped
```

### 15.2 Environment Variables (`.env`)

```bash
# Security
FINWISE_SECRET_KEY=change-this-to-a-random-string
FINWISE_PASSWORD=optional-password-for-basic-auth

# LLM Provider
LLM_PROVIDER=openai
LLM_API_KEY=your-api-key-here
LLM_MODEL=gpt-4o
# LLM_BASE_URL=http://localhost:11434/v1  # For Ollama

# Actual Budget
ACTUAL_BUDGET_URL=https://your-actual-budget-instance.com
ACTUAL_BUDGET_PASSWORD=your-password

# Database (optional)
DATABASE_URL=sqlite:///data/finwise.db
# DATABASE_URL=postgresql://user:pass@localhost/finwise

# Feature Toggles
AUTO_SYNC_THRESHOLD=1.0
STREAMING_MAX_TRANSACTIONS=30
```

### 15.3 First-Run Behavior

1. If `.env` is missing or `FINWISE_SECRET_KEY` is default, redirect to `/setup`.
2. Setup wizard collects LLM provider, API key, AB URL, and password.
3. Wizard tests connections (LLM ping + AB login + category fetch).
4. On success, wizard writes `.env` and initializes DB (runs Alembic migrations).
5. Learning Service seeds `payee_mappings` from AB history.
6. Redirect to dashboard.

---

## 16. Resolution of Reviewer Concerns

The following issues were identified in the CEO plan adversarial review. This architecture explicitly resolves them.

### 16.1 Critical (All Resolved)

| # | Concern | Resolution | Status |
|---|---------|------------|--------|
| 1 | **Latency target unrealistic** | Adjusted first-transaction target to **10 seconds** for statements with <30 transactions. This accounts for GPT-4o vision latency + network + SSE encoding. | ✅ Documented |
| 2 | **Accuracy threshold boundary** | Set **target accuracy to 85%** and **OCR trigger at <80%**. This creates a 5-point buffer, eliminating undefined behavior at exactly 80%. | ✅ Documented |
| 3 | **Narration delivery contradiction** | Clarified: The LLM returns narration in a JSON field. The **server** extracts this field and emits it as a standalone SSE `narration` event before transaction events. | ✅ Documented |
| 4 | **Data model contradiction** | Clarified: `pending_transactions.actual_budget_id` is a **foreign reference pointer** (the ID returned by AB's API), not duplicated transactional data. FinWise does not duplicate AB data. | ✅ Documented |

### 16.2 Recommended (All Resolved)

| # | Concern | Resolution | Status |
|---|---------|------------|--------|
| 5 | **Define `N` for top-N payee mappings** | Set **N = 10** (top-10 by frequency). This balances context window usage with relevance. | ✅ Documented |
| 6 | **Concurrent upload handling** | Implemented **max 1 active job per session**. New uploads during an active job receive `409 Conflict` with a user-friendly message. | ✅ Documented |
| 7 | **Address cold start** | Learning Service automatically seeds `payee_mappings` from Actual Budget's existing transaction history on first run. | ✅ Documented |
| 8 | **Document HTMX swap strategy** | Documented inline edit strategy: `hx-target="#tx-{id}"` and `hx-swap="innerHTML"` for zero-page-reload updates. | ✅ Documented |
| 9 | **Add deduplication strategy** | Added `dedup_hash` column (SHA-256 of date + payee + amount + upload_id). Checked before every insert. | ✅ Documented |
| 10 | **Define archiving strategy** | Implemented nightly archiving: soft-delete or move `synced` transactions older than 90 days to `archived_transactions`. | ✅ Documented |
| 11 | **Add rate limiting for Actual Budget API** | Implemented token-bucket rate limiter: **max 10 sync calls/minute**, exponential backoff on `429`, max 3 retries before user notification. | ✅ Documented |

## 17. Engineering Review TODOs

The following items were identified during `/plan-eng-review` and must be applied before implementation:

| # | File | Section | Issue | Fix | Status |
|---|------|---------|-------|-----|--------|
| 1 | `DESIGN.md` | 11.1 | DaisyUI v4 config uses `tailwind.config.js` (v3 syntax) but stack is Tailwind v4 | Update to CSS-based DaisyUI v4 theme config: `@plugin "daisyui/theme"` | ✅ Applied |
| 2 | `DESIGN.md` | 11.3 | Theme transition applies to ALL elements: `html, html * { transition: ... }` | Scope transition to `html` only | ✅ Applied |
| 3 | `DESIGN.md` | 2.1/15.4 | Muted text `#A1A1AA` on `#FFFFFF` is 3.1:1 — fails WCAG AA | Change to `#71717A` (4.6:1) | ✅ Applied |
| 4 | `ARCHITECTURE.md` | 8.1 | Chat pagination missing — loads ALL messages | Add `GET /chat/messages?limit=50&before_id={id}` endpoint | Pending |
| 5 | `ARCHITECTURE.md` | 8.1 / 13.1 | `dedup_hash = SHA256(date+payee+amount)` collides for recurring purchases | Include `upload_id` or `source_filename` in hash input | ✅ Applied |
| 6 | `ARCHITECTURE.md` | 10.1 | LLM client is custom wrapper over 3 provider SDKs | Adopt `litellm` library instead of custom wrapper | Pending |
| 7 | `ARCHITECTURE.md` | 6.1 | Full Repository layer proposed for 4 tables | Drop Repository layer; Services query SQLAlchemy models directly | ✅ Applied |
| 8 | `ARCHITECTURE.md` | 8.3 | SSE resume logic queries DB per event without index | Add composite index on `pending_transactions(job_id, created_at)` | ✅ Applied |
| 9 | `ARCHITECTURE.md` | 13.1 | Active job per session tracked implicitly | Add `active_job_id` to `chat_sessions` | ✅ Applied |
| 10 | `ARCHITECTURE.md` | 7.3 | No index on `pending_transactions.confidence` | Add index on `(confidence, status, created_at)` for calibration queries | ✅ Applied |

---

## 18. Technology Stack Summary

| Layer | Technology |
|-------|------------|
| **Backend** | Python 3.11+, FastAPI, Uvicorn |
| **Frontend** | HTMX, Jinja2, DaisyUI, Tailwind CSS v4 |
| **Database** | SQLite (default), PostgreSQL (optional), SQLAlchemy 2.0, Alembic |
| **LLM** | Custom wrapper: OpenAI SDK, Anthropic SDK, raw HTTP for Ollama |
| **OCR** | Tesseract (primary), EasyOCR (evaluative fallback) |
| **File Parsing** | Pandas (CSV/XLSX), ofxparse (OFX) |
| **Encryption** | Fernet (cryptography library) |
| **Deployment** | Docker, Docker Compose, `.env` configuration |

---

*End of Architecture Document*
