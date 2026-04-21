# FinWise — Phase 1 (MVP) Design Specification

**Date:** 2025-04-21  
**Status:** Approved  
**Scope:** Core Platform + Bookkeeper AI Persona

---

## 1. Overview

FinWise is an AI-powered assistant for self-hosted Actual Budget instances. Phase 1 delivers a chat-based interface (the Bookkeeper) that lets users add transactions via natural language, receipt images, credit card statement screenshots, and bulk file uploads (CSV, XLSX, OFX, TXT).

**Key Principles:**
- Fully self-hosted and open source
- User provides their own LLM API key (OpenAI, Anthropic, Ollama, etc.)
- User provides their own Actual Budget instance credentials
- No data sent to FinWise servers — all processing is local
- Modular architecture so Actual Budget can be replaced by a native engine in the future

---

## 2. Architecture

```
┌─────────────────────────────────────────┐
│           User Browser                  │
│  (HTMX + DaisyUI/Tailwind CSS)          │
└─────────────┬───────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│         FastAPI Backend                 │
│  ┌──────────┐  ┌──────────┐  ┌──────┐ │
│  │  Chat    │  │ Upload   │  │ Auth │ │
│  │  Router  │  │ Router   │  │ etc  │ │
│  └────┬─────┘  └────┬─────┘  └──────┘ │
│       │             │                  │
│  ┌────┴─────────────┴────────────────┐ │
│  │         Service Layer              │ │
│  │  ┌──────────┐  ┌────────────────┐ │ │
│  │  │ Bookkeeper│  │ Actual Budget  │ │ │
│  │  │  Service  │  │    Client      │ │ │
│  │  │ (LLM API) │  │  (API wrapper) │ │ │
│  │  └──────────┘  └────────────────┘ │ │
│  └────────────────────────────────────┘ │
│       │                                 │
│  ┌────┴────┐  ┌─────────────────────┐  │
│  │ SQLite  │  │  File Storage       │  │
│  │  (data) │  │  (receipt images)   │  │
│  └─────────┘  └─────────────────────┘  │
└─────────────────────────────────────────┘
```

**Technology Stack:**
- **Backend:** FastAPI (Python 3.11+)
- **Frontend interactivity:** HTMX
- **Templating:** Jinja2
- **Styling:** DaisyUI + Tailwind CSS v4
- **Database:** SQLite (default), PostgreSQL (optional)
- **File storage:** Local filesystem

---

## 3. Core Features

| Feature | Description |
|---------|-------------|
| **Chat UI** | Full-screen chat interface for the Bookkeeper persona. Supports text and file uploads. |
| **Natural Language Transactions** | "I spent $45 at Starbucks" → parsed → preview → confirm → synced to Actual Budget. |
| **Receipt Upload** | Image (JPG/PNG) → LLM vision extracts data → structured preview → confirm. |
| **Credit Card Statement Screenshots** | Screenshot → LLM vision extracts multiple transactions → batch preview → confirm all or edit individual. |
| **Bulk File Upload** | CSV / XLSX / TXT / OFX from banking apps → parsed → batch preview → confirm all or edit individual. |
| **Transaction History** | Last N transactions pulled from Actual Budget API and displayed in a sortable table. |
| **Mini Dashboard** | This month's spending, top categories, recent activity. |
| **Settings** | LLM provider configuration, Actual Budget connection, preferences. |
| **First-Run Setup Wizard** | Guided onboarding on first launch: LLM setup → Actual Budget connection → test → ready. |

### 3.1 Out of Scope (Phase 1)

- Other AI personas (Analyst, Spending Consultant, Planner)
- Browser extension for Amazon/Uber scraping
- Standalone scrapers
- Full custom dashboard with AI-generated recommendations
- Standalone budget engine (replacing Actual Budget)
- Multi-user support
- Advanced authentication (OAuth, SSO)

---

## 4. Data Flow

### 4.1 Natural Language Transaction

```
User types: "Lunch at Chipotle, $12.50"
        │
        ▼
┌─────────────────┐
│  FastAPI Route  │─── Validates input
│   /chat/send    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Bookkeeper      │─── Sends to LLM with system prompt
│ Service         │    "Extract: amount, payee, category, date, notes"
│ (LLM API)       │
└────────┬────────┘
         │
    ┌────┴────┐
    ▼         ▼
Confirmed  Missing info
    │         │
    ▼         ▼
Actual      Return to user
Budget      with question
API
    │
    ▼
Return success
    │
    ▼
HTMX swaps chat UI
```

### 4.2 File Upload (Unified)

```
User uploads file (receipt.jpg / statement.png / transactions.csv / data.ofx)
        │
        ▼
┌─────────────────┐
│  Upload Router  │─── Detects file type by extension & MIME
│   /upload       │
└────────┬────────┘
         │
    ┌────┴────┬────────────┐
    ▼         ▼            ▼
  Image    Spreadsheet    OFX/TXT
    │         │            │
    ▼         ▼            ▼
┌───────┐  ┌────────┐  ┌────────┐
│ LLM   │  │ Pandas │  │ OFX    │
│ Vision│  │ Parser │  │ Parser │
└───┬───┘  └───┬────┘  └───┬────┘
    │          │           │
    └────┬─────┴─────┬─────┘
         ▼           ▼
┌─────────────────────────────────┐
│      Transaction Preview        │
│  ┌─────────────────────────┐    │
│  │ Date       Payee    Amt │    │
│  │ 2024-01-15 Chipotle $12 │ ✅ │
│  │ 2024-01-15 Netflix  $15 │ ✅ │
│  │ 2024-01-16 Amazon   $34 │ ⚠️ │
│  └─────────────────────────┘    │
│      [Confirm All] [Edit]       │
└─────────────────────────────────┘
         │
    ┌────┴────┐
    ▼         ▼
 Confirmed   Edited
    │          │
    ▼          ▼
 Actual    Re-parse
 Budget    with edits
 API
```

---

## 5. Database Schema

Using SQLite by default. PostgreSQL supported via `DATABASE_URL`.

### 5.1 Tables

**`settings`**
```sql
id INTEGER PRIMARY KEY
actual_budget_url TEXT
actual_budget_password TEXT  -- encrypted at rest
llm_provider TEXT            -- openai, anthropic, ollama, etc.
llm_api_key TEXT             -- encrypted
llm_model TEXT               -- gpt-4o, claude-3.5-sonnet, etc.
default_currency TEXT        -- USD, BRL, etc.
created_at TIMESTAMP
updated_at TIMESTAMP
```

**`chat_sessions`**
```sql
id INTEGER PRIMARY KEY
persona TEXT                 -- 'bookkeeper', 'analyst', etc.
created_at TIMESTAMP
```

**`messages`**
```sql
id INTEGER PRIMARY KEY
session_id INTEGER FK
role TEXT                    -- 'user', 'assistant', 'system'
content TEXT
attachments JSON             -- [{filename, type, url}]
created_at TIMESTAMP
```

**`pending_transactions`**
```sql
id INTEGER PRIMARY KEY
message_id INTEGER FK
date DATE
payee TEXT
amount DECIMAL
category TEXT                -- mapped to Actual Budget category
notes TEXT
status TEXT                  -- 'pending', 'confirmed', 'rejected', 'edited'
source TEXT                  -- 'nlp', 'receipt', 'screenshot', 'csv', 'ofx'
raw_data JSON                -- original parsed data for debugging
created_at TIMESTAMP
```

**`sync_log`**
```sql
id INTEGER PRIMARY KEY
pending_transaction_id INTEGER FK
actual_budget_transaction_id TEXT
sync_status TEXT             -- 'success', 'failed'
error_message TEXT
synced_at TIMESTAMP
```

### 5.2 Design Choice

We do NOT duplicate Actual Budget's transaction data. We store only:
- Pending transactions (before they go to AB)
- Sync log (what we sent, what AB returned)
- Messages (chat history)

All reporting and history queries hit Actual Budget's API directly.

---

## 6. API Design

### 6.1 FastAPI Routes

| Route | Method | Description |
|-------|--------|-------------|
| `GET /` | — | Dashboard (mini overview + chat widget) |
| `GET /chat` | — | Full chat interface |
| `POST /chat/send` | HTMX | Send message → return AI response HTML |
| `POST /chat/upload` | HTMX | Upload file → return preview HTML |
| `POST /transactions/confirm` | HTMX | Confirm pending transaction(s) |
| `POST /transactions/edit` | HTMX | Edit pending transaction |
| `POST /transactions/reject` | HTMX | Reject pending transaction |
| `GET /history` | — | Transaction history from Actual Budget |
| `GET /settings` | — | Settings page |
| `POST /settings` | Form | Save settings |
| `GET /setup` | — | First-run setup wizard |
| `POST /setup` | Form | Complete setup |

### 6.2 LLM Integration

Provider-agnostic wrapper supporting OpenAI, Anthropic, Ollama, and any OpenAI-compatible API.

```python
class LLMClient:
    def __init__(self, provider: str, api_key: str, model: str):
        self.provider = provider
        self.client = self._get_client(api_key)
        self.model = model
    
    def chat(self, messages: list, json_mode: bool = False) -> str: ...
    
    def chat_with_image(self, messages: list, image_bytes: bytes) -> str: ...
```

### 6.3 Bookkeeper System Prompt

```markdown
You are the Bookkeeper, a helpful assistant for managing personal finances.

Your tasks:
1. Parse user messages into structured transactions
2. Extract data from receipt images and bank statements
3. Ask for missing required fields: amount, payee, date, category
4. Suggest categories based on payee and user history
5. Always respond with JSON when asked to parse data

Available categories: {{ categories }}
Default account: {{ default_account }}

Return format for transactions:
{
  "transactions": [
    {
      "date": "YYYY-MM-DD",
      "payee": "string",
      "amount": 0.00,
      "category": "string",
      "notes": "string"
    }
  ],
  "missing_info": ["field_name"],
  "message": "friendly response to user"
}
```

---

## 7. UI Structure

### 7.1 Pages

| Page | Route | Description |
|------|-------|-------------|
| **Dashboard** | `/` | Spending summary, recent transactions, quick chat widget |
| **Chat** | `/chat` | Full-screen chat with Bookkeeper |
| **History** | `/history` | Transaction list with filters |
| **Settings** | `/settings` | API keys, Actual Budget config, preferences |
| **Setup** | `/setup` | First-run wizard |

### 7.2 Jinja2 Macros

```html
<!-- components/chat.html -->
{% macro chat_bubble(content, role) %}
  <div class="chat chat-{{ role }}">
    <div class="chat-bubble">{{ content }}</div>
  </div>
{% endmacro %}

<!-- components/transaction_preview.html -->
{% macro transaction_preview(transactions) %}
  <div class="card bg-base-200">
    <div class="card-body">
      <h3 class="card-title">Pending Transactions</h3>
      <table class="table table-zebra">...</table>
      <button class="btn btn-primary" hx-post="/transactions/confirm">Confirm All</button>
    </div>
  </div>
{% endmacro %}

<!-- components/stat_card.html -->
{% macro stat_card(title, value, change) %}
  <div class="stat bg-base-200 rounded-box">
    <div class="stat-title">{{ title }}</div>
    <div class="stat-value">{{ value }}</div>
    {% if change %}<div class="stat-desc">{{ change }}</div>{% endif %}
  </div>
{% endmacro %}
```

### 7.3 Base Layout

```html
<!DOCTYPE html>
<html data-theme="light">
<head>
  <script src="/static/htmx.min.js"></script>
  <link rel="stylesheet" href="/static/daisyui.css">
  <link rel="stylesheet" href="/static/tailwind.css">
</head>
<body>
  <div class="drawer lg:drawer-open">
    <input id="sidebar" type="checkbox" class="drawer-toggle">
    <div class="drawer-content">
      {% block content %}{% endblock %}
    </div>
    <div class="drawer-side">
      <!-- Sidebar navigation -->
    </div>
  </div>
</body>
</html>
```

---

## 8. Error Handling

| Scenario | Behavior |
|----------|----------|
| LLM API fails / invalid key | User-friendly error: "Could not connect to AI service. Check your API key in Settings." Log to console. |
| Actual Budget API fails | Queue transaction for retry. Notify user: "Actual Budget is unavailable. Transaction saved and will retry." |
| Invalid file upload | Reject with clear message: "Unsupported file type. Please upload JPG, PNG, CSV, XLSX, OFX, or TXT." |
| LLM returns malformed JSON | Show raw response to user with "I couldn't parse this. Here's what I got — can you help?" |
| Transaction validation fails | Highlight offending field in preview. Allow user to edit inline. |

---

## 9. Security

| Concern | Mitigation |
|---------|------------|
| API key storage | Encrypt at rest using Fernet (symmetric encryption with key from `FINWISE_SECRET_KEY`). Keys never logged. |
| Actual Budget password | Same encryption. Stored only in settings DB. |
| File uploads | Validate MIME type and extension. Save outside web root. Max 10MB. |
| Session / auth | Single-user app. Optional basic auth via env var (`FINWISE_PASSWORD`). |
| HTTPS | Required for production. Document reverse proxy setup (Caddy/Nginx). |
| LLM data privacy | User controls LLM provider. Local Ollama supported for full privacy. |

---

## 10. Deployment & Self-Hosting

### 10.1 Requirements

- Docker (recommended) or Python 3.11+
- 512MB RAM minimum (1GB recommended)
- 1GB disk (SQLite + uploaded files)

### 10.2 Docker Compose

```yaml
version: '3.8'
services:
  finwise:
    image: finwise:latest
    ports:
      - "8000:8000"
    env_file:
      - .env
    volumes:
      - ./data:/app/data
      - ./uploads:/app/uploads
```

### 10.3 Environment Variables (`.env`)

| Variable | Description |
|----------|-------------|
| `FINWISE_SECRET_KEY` | Encryption key for API keys |
| `FINWISE_PASSWORD` | Optional basic auth password |
| `LLM_PROVIDER` | `openai`, `anthropic`, `ollama` |
| `LLM_API_KEY` | API key for chosen provider |
| `LLM_MODEL` | Model name |
| `ACTUAL_BUDGET_URL` | Actual Budget instance URL |
| `ACTUAL_BUDGET_PASSWORD` | Actual Budget password |
| `DATABASE_URL` | `sqlite:///data/finwise.db` or PostgreSQL |

A `.env.example` file will be provided in the repository.

### 10.4 First-Run Flow

1. User opens FinWise for the first time
2. Redirected to `/setup` wizard
3. Step 1: Choose LLM provider + enter API key
4. Step 2: Enter Actual Budget connection details
5. Step 3: Test connection → fetch categories → confirm
6. Redirect to dashboard, ready to use

---

## 11. Future Phases

| Phase | Scope |
|-------|-------|
| **Phase 2** | Analyst persona (spending analysis, charts, trends), Spending Consultant persona (purchase rationalization) |
| **Phase 3** | Planner persona (budget creation, goals), enhanced dashboard with AI recommendations |
| **Phase 4** | Browser extension for Amazon/Uber scraping, standalone scrapers |
| **Phase 5** | Standalone budget engine (no Actual Budget dependency) |

---

## 12. ADRs

Architecture Decision Records documenting key decisions are stored in `docs/adr/`:

- `001-tech-stack.md` — FastAPI + HTMX + DaisyUI/Tailwind CSS
- `002-llm-integration.md` — Provider-agnostic LLM wrapper
- `003-data-storage.md` — SQLite default, PostgreSQL optional
- `004-file-upload-formats.md` — Supported import formats
- `005-deployment-configuration.md` — Docker + .env-based configuration
