# FinWise

AI-native personal finance platform that extracts transactions from credit card statement screenshots and syncs them to [Actual Budget](https://actualbudget.org/).

## Features

- **Screenshot to transactions:** Upload a credit card statement screenshot and watch transactions appear in real-time
- **HTMX + SSE chat interface:** Review, edit, and confirm transactions inline
- **Learning payee mappings:** The system learns your payee -> category preferences from corrections
- **Actual Budget sync:** Confirmed transactions sync to your Actual Budget instance
- **Multi-provider LLM support:** OpenAI, Anthropic, Ollama via `litellm`
- **File parsing:** CSV, XLSX, OFX, TXT, and image uploads
- **Self-hosted:** Single `docker compose up` deployment

## Prerequisites

- Python 3.11+
- [Actual Budget](https://actualbudget.org/) instance running
- LLM API key (OpenAI, Anthropic, or local Ollama)

## Quick Start

### 1. Clone and configure

```bash
git clone <repo>
cd finwise
cp .env.example .env
# Edit .env with your credentials
```

### 2. Create virtual environment and install

```bash
python -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

### 3. Run database migrations

```bash
alembic upgrade head
```

### 4. Start the server

```bash
uvicorn src.main:app --reload --host 0.0.0.0 --port 8000
```

Visit http://localhost:8000/setup to configure your LLM and Actual Budget credentials.

## Docker

```bash
docker compose up
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `FINWISE_SECRET_KEY` | Yes | - | Min 32 chars, used for encryption |
| `FINWISE_PASSWORD` | No | - | Optional basic auth password |
| `LLM_PROVIDER` | Yes | `openai` | openai / anthropic / ollama |
| `LLM_API_KEY` | Yes | - | Your LLM API key |
| `LLM_MODEL` | Yes | `gpt-4o` | Model name |
| `LLM_BASE_URL` | No | - | For Ollama / custom endpoints |
| `ACTUAL_BUDGET_URL` | Yes | - | Your AB instance URL |
| `ACTUAL_BUDGET_PASSWORD` | Yes | - | AB server password |
| `DATABASE_URL` | No | `sqlite:///data/finwise.db` | Database connection |
| `AUTO_SYNC_THRESHOLD` | No | `1.0` | 1.0 = manual confirm for all |
| `STREAMING_MAX_TRANSACTIONS` | No | `30` | Auto-fallback for large statements |

## Tech Stack

- **Backend:** FastAPI, SQLAlchemy 2.0, Alembic, Pydantic
- **LLM:** litellm (multi-provider)
- **Frontend:** HTMX, Jinja2, DaisyUI + Tailwind CSS v4
- **Database:** SQLite (default), PostgreSQL (optional)
- **OCR:** Tesseract (optional fallback)

## Development

```bash
# Run tests
pytest

# Run linter
ruff check src/

# Build CSS (requires Node.js)
npm install
npm run build:css
```

## Project Structure

```
finwise/
├── src/
│   ├── main.py              # FastAPI app entrypoint
│   ├── config.py            # Pydantic settings
│   ├── database.py          # SQLAlchemy engine + session
│   ├── models/              # SQLAlchemy ORM models
│   ├── schemas/             # Pydantic request/response models
│   ├── services/            # Business logic (LLM, Bookkeeper, AB client, etc.)
│   ├── routers/             # FastAPI route handlers
│   ├── parsers/             # File format parsers
│   ├── templates/           # Jinja2 templates
│   └── utils/               # Shared utilities
├── static/                  # CSS, JS, assets
├── migrations/              # Alembic migrations
├── tests/                   # Pytest test suite
└── docker-compose.yml
```

## Architecture

FinWise follows a service-oriented architecture:

- **Services query SQLAlchemy models directly** (no Repository layer abstraction)
- **SSE streams** deliver real-time extraction progress to the chat interface
- **Fernet encryption** secures API keys and passwords at rest
- **Rate limiting** protects the Actual Budget API (max 10 calls/min with exponential backoff)

## License

MIT
