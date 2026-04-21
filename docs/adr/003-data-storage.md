# ADR 003: Data Storage — SQLite Default, PostgreSQL Optional

**Status:** Accepted  
**Date:** 2025-04-21  
**Deciders:** Project owner (Gabriel) + AI assistant

---

## Context

FinWise needs to store:
- App settings (LLM credentials, Actual Budget connection)
- Chat history (messages and sessions)
- Pending transactions (before confirmation)
- Sync logs (what was sent to Actual Budget)

The storage solution must be simple for self-hosting but robust enough for production use.

## Constraints

- Must work out of the box with zero configuration
- Must be self-hosted (no cloud database requirement)
- Should support backup and portability
- Should scale to at least thousands of transactions
- Must encrypt sensitive data (API keys, passwords)

## Alternatives Considered

### Alternative A: PostgreSQL Only

**Description:** Require PostgreSQL as the sole database.

**Pros:**
- Production-grade reliability
- Better concurrency
- Advanced features (JSONB, full-text search)

**Cons:**
- Requires separate PostgreSQL installation
- More complex Docker Compose setup
- Overkill for a single-user app
- Adds barrier to entry for non-technical users

**Rejected because:** Violates the "zero setup" constraint. A self-hosted tool should not require a separate database server for basic use.

### Alternative B: JSON Files on Disk

**Description:** Store data as JSON files in a data directory.

**Pros:**
- Ultimate simplicity
- Human-readable
- No database engine needed

**Cons:**
- No ACID guarantees
- No querying capabilities
- Race conditions with concurrent access
- Hard to maintain schema migrations
- Doesn't scale

**Rejected because:** Too primitive for any real usage. We'd end up rebuilding SQLite poorly.

### Alternative C: SQLite Default, PostgreSQL Optional

**Description:** Use SQLite as the default database. Allow advanced users to opt into PostgreSQL via `DATABASE_URL`.

**Pros:**
- Zero configuration — file-based, ships with Python
- Perfect for single-user or low-concurrency apps
- Portable — just copy the `.db` file
- SQLAlchemy supports both with the same code
- Easy backup

**Cons:**
- Not ideal for high concurrency (but this is a single-user app)
- Limited write performance (irrelevant for chat + occasional transactions)

**Accepted because:** Best fit for a self-hosted, single-user MVP. The optional PostgreSQL path ensures we don't paint ourselves into a corner.

## Decision

Use **SQLite as the default database** with **PostgreSQL as an optional alternative** via `DATABASE_URL` environment variable.

## Tradeoffs

- **Simplicity vs scalability:** We optimize for simplicity and zero-setup. If a user needs to scale beyond SQLite's limits, PostgreSQL is a config change away.
- **File-based vs server-based:** SQLite is a file — easy to back up, but harder to share across multiple app instances. This is acceptable for a self-hosted single-node deployment.

## Implementation Approach

1. Use SQLAlchemy ORM with Alembic for migrations
2. Default `DATABASE_URL=sqlite:///data/finwise.db`
3. User can override with `DATABASE_URL=postgresql://...`
4. All schemas work on both databases (avoid SQLite-specific features)
5. Encrypt sensitive fields (API keys, passwords) at the application layer using Fernet

## Consequences

- **Positive:** Works immediately with no setup. Perfect for Docker deployments. Easy backups.
- **Negative:** If we later add multi-user support or real-time collaboration, we'll need to recommend PostgreSQL. This is expected and documented.
