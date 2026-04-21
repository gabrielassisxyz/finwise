# ADR 005: Deployment Configuration — Docker + .env-Based Config

**Status:** Accepted  
**Date:** 2025-04-21  
**Deciders:** Project owner (Gabriel) + AI assistant

---

## Context

FinWise must be fully self-hosted. Users should be able to deploy it with minimal effort. Configuration should be organized and not require modifying Docker Compose files.

## Constraints

- Must work with Docker Compose
- Must be easy for non-technical users to configure
- Must keep secrets out of version control
- Must support both quick-start and advanced configurations
- Should follow 12-factor app principles

## Alternatives Considered

### Alternative A: Environment Variables in docker-compose.yml Only

**Description:** Define all configuration directly in `docker-compose.yml` under `environment:`.

**Pros:**
- Everything in one file
- Simple for Docker-savvy users

**Cons:**
- Secrets committed to version control (if user modifies compose file)
- Harder to share configurations
- Messy file with many env vars
- User must edit YAML (error-prone)

**Rejected because:** Doesn't follow best practices for secret management. Makes it too easy to accidentally commit credentials.

### Alternative B: Config File (JSON/YAML) Mounted as Volume

**Description:** Use a `config.yaml` or `config.json` file mounted into the container.

**Pros:**
- Structured configuration
- Easy to read and edit
- Supports comments (YAML)

**Cons:**
- Another file format to learn
- Must handle file permissions
- Not standard for Docker deployments
- Still risk of committing secrets

**Rejected because:** Adds complexity. Environment variables are the Docker standard.

### Alternative C: .env File with docker-compose.yml Reference

**Description:** Store all configuration in a `.env` file. `docker-compose.yml` references it via `env_file: - .env`. Provide `.env.example` as a template.

**Pros:**
- `.env` is standard and familiar
- `.gitignore` prevents committing secrets
- `.env.example` shows all options with defaults
- Clean separation: compose file = infrastructure, .env = configuration
- Easy to backup and version config (minus secrets)

**Cons:**
- User must create `.env` from `.env.example`
- One more step in setup

**Accepted because:** Best balance of simplicity, security, and Docker best practices. The `.env.example` template makes onboarding easy.

## Decision

Use a **`.env` file for all configuration**, referenced by `docker-compose.yml`. Provide `.env.example` in the repository.

## Tradeoffs

- **Security vs convenience:** `.env` is not encrypted, but it's local to the user's machine. For a self-hosted app, this is standard practice. Users can use Docker secrets or vaults for advanced setups.
- **One file vs many:** We keep everything in one `.env` for simplicity. If the app grows, we may split into `.env` and mounted config files.

## Implementation Approach

### docker-compose.yml

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
    restart: unless-stopped
```

### .env.example

```bash
# FinWise Configuration
# Copy this file to .env and fill in your values

# Security
FINWISE_SECRET_KEY=change-this-to-a-random-string
FINWISE_PASSWORD=optional-password-for-basic-auth

# LLM Provider (openai, anthropic, ollama, openai-compatible)
LLM_PROVIDER=openai
LLM_API_KEY=your-api-key-here
LLM_MODEL=gpt-4o

# For Ollama or custom endpoints
# LLM_BASE_URL=http://localhost:11434/v1

# Actual Budget Connection
ACTUAL_BUDGET_URL=https://your-actual-budget-instance.com
ACTUAL_BUDGET_PASSWORD=your-actual-budget-password

# Database (optional — defaults to SQLite)
# DATABASE_URL=sqlite:///data/finwise.db
# DATABASE_URL=postgresql://user:pass@localhost/finwise
```

### .gitignore

```
.env
data/
uploads/
```

### First-Run Behavior

1. If `.env` is missing or `FINWISE_SECRET_KEY` is default, redirect to `/setup`
2. Setup wizard collects all required values
3. Wizard writes `.env` file and initializes database
4. Subsequent restarts read from `.env`

## Consequences

- **Positive:** Standard, secure, easy to understand. `.env.example` acts as documentation.
- **Negative:** Users must remember to create `.env` from `.env.example`. We mitigate this with the setup wizard.
