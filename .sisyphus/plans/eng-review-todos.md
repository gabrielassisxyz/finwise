# FinWise Eng Review TODOs — Implementation Plan

**Source:** DESIGN.md Section 18 (10 engineering review findings)  
**Branch:** `fix/eng-review-todos` (create before starting)  
**Created:** 2026-04-21  
**Status:** Ready for execution via `$ultrawork`

---

## Quick Reference: The 10 TODOs

| # | File | Section | Issue | Fix |
|---|------|---------|-------|-----|
| 1 | `DESIGN.md` | 11.1 | DaisyUI v4 config uses `tailwind.config.js` (v3 syntax) but stack is Tailwind v4 | Update to CSS-based DaisyUI v4 theme config: `@plugin "daisyui/theme"` |
| 2 | `DESIGN.md` | 11.3 | Theme transition applies to ALL elements: `html, html * { transition: ... }` | Scope transition to `html` only, or specific CSS custom properties |
| 3 | `DESIGN.md` | 11.3 | Muted text `#A1A1AA` on `#FFFFFF` is 3.1:1 — fails WCAG AA | Change to `#71717A` (4.6:1) or use only for large text |
| 4 | `ARCHITECTURE.md` | 8.1 | Chat pagination missing — loads ALL messages | Add `GET /chat/messages?limit=50&before_id={id}` endpoint |
| 5 | `ARCHITECTURE.md` | 8.1 / 13.1 | `dedup_hash = SHA256(date+payee+amount)` collides for recurring purchases | Include `upload_id` or `source_filename` in hash input |
| 6 | `ARCHITECTURE.md` | 10.1 | LLM client is custom wrapper over 3 provider SDKs | Adopt `litellm` library instead of custom wrapper |
| 7 | `ARCHITECTURE.md` | 6.1 | Full Repository layer proposed for 4 tables | Drop Repository layer; Services query SQLAlchemy models directly |
| 8 | `ARCHITECTURE.md` | 8.3 | SSE resume logic queries DB per event without index | Add composite index on `pending_transactions(job_id, created_at)` |
| 9 | `ARCHITECTURE.md` | 13.1 | Active job per session tracked implicitly | Add `active_job_id` to `chat_sessions` or use DB query with `status='pending'` |
| 10 | `ARCHITECTURE.md` | 7.3 | No index on `pending_transactions.confidence` | Add index on `(confidence, status, created_at)` for calibration queries |

---

## Wave Dependency Graph

```
Wave 1 (Structural) ──────────────────────────────────────►
├─ #7  Drop Repository layer ────────┐
├─ #8  Index: (job_id, created_at)   │ These 4 can run in PARALLEL
├─ #9  Add active_job_id             │ after #7 completes, OR
└─ #10 Index: (confidence, status)   │ in parallel with #7 if careful
                                     │
Wave 2 (Service Logic) ◄─────────────┘
├─ #5  Fix dedup hash collision
├─ #6  Adopt litellm
└─ #4  Add chat pagination endpoint  (depends on chat service being stable)

Wave 3 (Frontend/Config)
├─ #1  DaisyUI v4 CSS-based config
├─ #2  Theme transition scope fix
└─ #3  Muted text contrast fix       ALL 3 are independent, can run in parallel
```

---

## Execution Waves

### WAVE 1: Data Layer Restructure + DB Schema
**Can run in parallel:** Partially. #7 is a structural refactor that touches many files. #8, #9, #10 are DB migrations. If #7 drops repository files that #8-#10 would otherwise need to update, do #7 first. If #8-#10 only touch models/migrations, they can run in parallel with #7.

**Agent allocation:**
- `category="deep"` for #7 (structural refactor)
- `category="quick"` for #8, #9, #10 (migrations + query updates)

#### Task 7 — Drop Repository Layer
**Why:** Over-abstraction. 4 tables don't need a Repository pattern; SQLAlchemy models are enough.

**Files to explore first:**
- Find all Repository classes: `grep -r "class.*Repository" src/`
- Find all service files importing repositories: `grep -r "Repository" src/services/`

**Implementation:**
1. Inline repository query methods into their respective Service classes
2. Delete repository files
3. Update service imports
4. Verify no other code references repositories

**QA:**
- `grep -r "Repository" src/` should return nothing
- App still starts (`python -m src.main` or equivalent)
- All existing tests pass

#### Task 8 — Add Composite Index `(job_id, created_at)`
**Why:** SSE resume logic queries DB per event; this index makes resume O(log n) instead of O(n).

**Files:**
- `pending_transactions` model/migration file
- SSE resume query in chat/streaming service

**Implementation:**
1. Add Alembic migration: `CREATE INDEX idx_pending_tx_job_created ON pending_transactions(job_id, created_at)`
2. Verify the SSE resume query uses this index (check query plan)

**QA:**
- Migration runs successfully (`alembic upgrade head`)
- `EXPLAIN` on the SSE resume query shows Index Scan

#### Task 9 — Explicit `active_job_id` Tracking
**Why:** Currently tracked implicitly (e.g., latest pending job). Explicit is clearer and race-condition-safe.

**Files:**
- `chat_sessions` model/migration
- Chat session service (where jobs are created/assigned)
- SSE/streaming service (where active job is queried)

**Implementation options:**
- **Option A (recommended):** Add `active_job_id` column to `chat_sessions`, nullable. Set on job creation, clear on completion.
- **Option B:** Query DB for `status='pending'` per session. Simpler but queries on every check.

**QA:**
- Migration succeeds
- Creating a chat job sets `active_job_id`
- Completing the job clears `active_job_id` (or sets to NULL)
- No implicit "latest job" logic remains in SSE resume path

#### Task 10 — Add Index on `(confidence, status, created_at)`
**Why:** Calibration queries ("show me pending items by confidence") currently table-scan.

**Files:**
- `pending_transactions` model/migration

**Implementation:**
1. Add Alembic migration: `CREATE INDEX idx_pending_tx_confidence ON pending_transactions(confidence, status, created_at)`

**QA:**
- Migration runs
- Calibration/dashboard queries use index scan

---

### WAVE 2: Service Logic + API
**Can run in parallel:** YES. #5, #6, and #4 are independent once Wave 1 is stable.

**Agent allocation:**
- `category="deep"` for #6 (litellm swap — library boundary change)
- `category="unspecified-high"` for #5 (dedup hash logic)
- `category="quick"` for #4 (endpoint addition)

#### Task 5 — Fix Dedup Hash Collision
**Why:** `SHA256(date+payee+amount)` collides for recurring purchases (same place, same amount, different day still works, but same day + same coffee shop = collision).

**Files:**
- Transaction processing service (where dedup_hash is computed)
- `pending_transactions` model (where `dedup_hash` is stored)
- Any code that queries by `dedup_hash`

**Implementation:**
```python
# BEFORE
dedup_hash = sha256(f"{date}{payee}{amount}".encode()).hexdigest()

# AFTER
# Include upload_id or source_filename to scope dedup to a single import
dedup_hash = sha256(f"{date}{payee}{amount}{upload_id}".encode()).hexdigest()
```

**Decision needed:** If `upload_id` is not available for manual entry, use a sentinel (e.g., `manual-{session_id}`). Document this in code.

**QA:**
- Two transactions with same date/payee/amount but different uploads get different hashes
- Same upload with duplicate row gets same hash (still deduped within upload)
- Existing tests pass (or update test expectations)

#### Task 6 — Adopt `litellm`
**Why:** Custom wrapper over 3 provider SDKs (OpenAI, Anthropic, Google) is maintenance burden. `litellm` provides unified interface.

**Files:**
- `src/llm/` or `src/services/llm_client.py` (find the custom wrapper)
- `requirements.txt` / `pyproject.toml`
- Any config referencing provider-specific settings

**Implementation:**
1. Add `litellm` to dependencies
2. Replace custom wrapper with `litellm.completion()` calls
3. Map existing config (model name, temperature, etc.) to litellm params
4. Remove provider SDK imports if no longer needed

**Caveat:** `litellm` may not support all features (e.g., streaming format, tool calling). Verify before full swap.

**QA:**
- `pip install litellm` (or add to requirements)
- LLM chat still works end-to-end
- All 3 providers still configurable via settings
- Streaming responses still work

#### Task 4 — Chat Pagination Endpoint
**Why:** Currently loads ALL messages. With long sessions this becomes OOM risk.

**Files:**
- Chat routes/controller (`src/routes/chat.py` or similar)
- Chat service (`src/services/chat.py`)
- Message model (`src/models/message.py`)

**Implementation:**
```python
@router.get("/chat/messages")
async def get_messages(
    limit: int = Query(50, ge=1, le=200),
    before_id: Optional[int] = Query(None),
    session_id: str = Query(...),  # or from auth context
):
    """Get messages before a given ID (for infinite scroll up)."""
    query = select(Message).where(Message.session_id == session_id)
    if before_id:
        query = query.where(Message.id < before_id)
    query = query.order_by(Message.id.desc()).limit(limit)
    # ... execute, reverse to chronological, return
```

**QA:**
- `GET /chat/messages?session_id=xxx&limit=10` returns latest 10
- `GET /chat/messages?session_id=xxx&before_id=100&limit=10` returns 10 before id 100
- Total count header included for UI pagination state

---

### WAVE 3: Frontend Config Fixes
**Can run in parallel:** YES. All 3 are independent CSS/config changes.

**Agent allocation:**
- `category="visual-engineering"` for all 3 (frontend config work)

#### Task 1 — DaisyUI v4 CSS-Based Config
**Why:** Tailwind v4 uses CSS-based config, not `tailwind.config.js`. DaisyUI v4 themes are declared via `@plugin`.

**Files:**
- `tailwind.config.js` (to be modified or removed)
- `src/styles.css` or `src/index.css` (where Tailwind directives live)

**Implementation:**
```css
/* In your main CSS file */
@import "tailwindcss";

@plugin "daisyui" {
  themes: light --default, dark;
}

@plugin "daisyui/theme" {
  name: "finwise";
  default: true;
  /* Map all tokens from DESIGN.md Section 11.1 */
  --color-primary: #2563EB;
  --color-secondary: #18181B;
  /* ... etc */
}
```

**QA:**
- Build succeeds (`npm run build` or `bun run build`)
- Light theme renders correctly
- Soft dark and OLED themes also defined

#### Task 2 — Theme Transition Scope Fix
**Why:** `html, html * { transition: ... }` causes every element to animate on theme switch — janky, CPU-heavy.

**Files:**
- Global CSS file (where theme transition is defined)

**Implementation:**
```css
/* BEFORE */
html, html * {
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

/* AFTER */
html {
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}
```

Or use CSS custom properties:
```css
:root {
  --transition-theme: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}
.theme-transition {
  transition: var(--transition-theme);
}
```

**QA:**
- Theme switch still smooth
- No jank on large pages
- `prefers-reduced-motion` respected

#### Task 3 — Muted Text Contrast Fix
**Why:** `#A1A1AA` on `#FFFFFF` is 3.1:1 — fails WCAG AA for normal text (needs 4.5:1).

**Files:**
- `DESIGN.md` (update token table)
- Global CSS / Tailwind config (update `text-zinc-400` mapping or custom token)
- Any component using muted text color

**Implementation:**
1. Change Muted token from `#A1A1AA` to `#71717A`
2. Update Tailwind/DaisyUI theme config
3. Audit all `text-zinc-400` (or equivalent) usages — they should automatically update if using tokens

**QA:**
- Contrast ratio >= 4.5:1 (use browser devtools or online checker)
- Muted text still visually distinct from secondary text

---

## Parallelization Summary

### Can Run in Parallel (same wave, no shared files):

**Wave 1:**
- #8, #9, #10 (all DB migrations — different indexes/columns)
- #7 with #8/#9/#10 ONLY IF repositories don't own the models. If repositories abstract model access, do #7 first.

**Wave 2:**
- #5 and #6 (different services entirely)
- #4 after #5/#6 are conceptually done (or if chat service is already stable)

**Wave 3:**
- #1, #2, #3 (all CSS/config, no shared mutable state)

### Must Run Sequentially:

1. **Wave 1 → Wave 2:** Service logic (#4, #5, #6) should run after data layer (#7-#10) is committed. Otherwise merge conflicts on service files.
2. **Wave 2 → Wave 3:** Not strictly required — frontend changes are independent. But doing backend first lets you test end-to-end.

### Recommended Agent Spawns for `$ultrawork`

```
# Session 1: Wave 1 (structural)
$ultrawork wave-1-repo-layer      # Task 7
$ultrawork wave-1-db-migrations   # Tasks 8, 9, 10 in parallel

# Session 2: Wave 2 (service logic)
$ultrawork wave-2-dedup-hash      # Task 5
$ultrawork wave-2-litellm         # Task 6
$ultrawork wave-2-chat-paginate   # Task 4

# Session 3: Wave 3 (frontend)
$ultrawork wave-3-daisyui-config  # Task 1
$ultrawork wave-3-theme-fixes     # Tasks 2, 3
```

---

## Verification Checklist (Per Wave)

**Before starting next wave:**
- [ ] All tasks in current wave committed to branch
- [ ] `git diff --stat` reviewed — no unexpected files touched
- [ ] App starts without errors
- [ ] Existing tests pass (or failures are pre-existing and documented)

**Final verification (after all waves):**
- [ ] All 10 TODOs marked DONE in DESIGN.md Section 18
- [ ] `git log --oneline` shows clean atomic commits
- [ ] No `as any`, `@ts-ignore`, or `@ts-expect-error` added
- [ ] No pre-existing tests deleted

---

## Handoff Notes

- **Context-save recommended** before starting ultrawork: `$context-save`
- **If a task is blocked** (e.g., litellm doesn't support a needed feature): document the blocker in the commit message, move to next task, ask user at end of wave.
- **Repository layer removal (#7)** is the riskiest task — it touches the most files. If it fails, the whole wave is blocked. Consider doing it first and alone.
- **DB migrations (#8, #9, #10)** can be combined into a single Alembic migration file for cleanliness.

---

*Generated by Sisyphus for ultrawork orchestration.*
