# ADR 001: Tech Stack — FastAPI + HTMX + DaisyUI/Tailwind CSS

**Status:** Accepted  
**Date:** 2025-04-21  
**Deciders:** Project owner (Gabriel) + AI assistant

---

## Context

FinWise is a self-hosted AI assistant for Actual Budget. The project owner is not familiar with JavaScript or frontend frameworks but is comfortable with Python. The stack must be:
- Simple and lightweight
- Fast to develop and iterate
- Easy to self-host
- Suitable for agentic coding (AI-assisted development)

## Constraints

- Project owner lacks frontend framework experience
- Must be fully self-hosted (no SaaS dependencies)
- Should support agentic coding efficiently
- Must handle chat UI, file uploads, dashboard, and settings
- Future standalone budget engine must fit the same architecture

## Alternatives Considered

### Alternative A: Next.js / React (Full-Stack)

**Description:** Single Next.js codebase with React frontend and API routes.

**Pros:**
- Rich ecosystem, many libraries
- Good for complex SPAs
- SSR/SSG built-in

**Cons:**
- Requires learning React, hooks, state management
- Heavy bundle size
- Overkill for a forms/chat/dashboard app
- Steep learning curve for the project owner

**Rejected because:** Violates the simplicity and lightweight constraints. The project owner is not familiar with JavaScript frameworks.

### Alternative B: FastAPI + Streamlit

**Description:** Python-only stack with Streamlit rendering the UI.

**Pros:**
- Everything in Python
- Rapid prototyping
- Great for data dashboards

**Cons:**
- Not suitable for custom chat interfaces
- Limited interactivity patterns
- Hard to style professionally
- Session state management is awkward

**Rejected because:** Streamlit is designed for data apps, not conversational/chat interfaces. It would fight us on custom UI needs.

### Alternative C: FastAPI + Simple React

**Description:** Python backend with a lightweight React frontend.

**Pros:**
- Industry standard
- Powerful component ecosystem
- Good separation of concerns

**Cons:**
- Requires learning React basics
- Build step needed (Vite, Webpack)
- State management complexity
- More files and abstractions to maintain

**Rejected because:** Adds unnecessary complexity for a single developer who doesn't know React. The learning curve doesn't justify the benefits for this project's scope.

### Alternative D: FastAPI + HTMX + Jinja2 + DaisyUI/Tailwind CSS

**Description:** Python backend renders HTML templates. HTMX adds interactivity via HTML attributes. DaisyUI provides pre-built components on top of Tailwind CSS.

**Pros:**
- All logic stays in Python
- No JavaScript framework to learn
- HTMX is ~14KB, zero dependencies
- DaisyUI gives Bootstrap-like simplicity with Tailwind's power
- Jinja2 macros make component reuse easy
- Perfect for agentic coding (semantic classes, less token usage)
- Chat components (bubbles, cards) built into DaisyUI

**Cons:**
- Not a true SPA (every interaction hits the server)
- For streaming AI responses, needs SSE or small JS snippet
- Smaller component ecosystem than React

**Accepted because:** Best aligns with constraints. The "server-rendered HTML + progressive enhancement" model is ideal for a single Python developer. DaisyUI specifically solves the "class soup" problem of raw Tailwind.

## Decision

Use **FastAPI + HTMX + Jinja2 + DaisyUI + Tailwind CSS v4** as the tech stack.

## Tradeoffs

- **Server load vs client complexity:** We accept more server requests in exchange for zero frontend framework complexity.
- **SEO vs interactivity:** Not a concern for a self-hosted tool.
- **Learning curve:** HTMX and DaisyUI can be learned in hours, not weeks.

## Implementation Approach

1. FastAPI serves Jinja2 templates
2. HTMX attributes (`hx-get`, `hx-post`, `hx-target`) handle interactivity
3. DaisyUI classes (`btn`, `card`, `chat-bubble`, `stat`) style components
4. Tailwind utilities used only for custom tweaks
5. Jinja2 macros abstract repeated components
6. SSE endpoint for streaming LLM responses in chat

## Consequences

- **Positive:** Fast development, easy maintenance, low cognitive load, excellent for agentic coding
- **Negative:** For Phase 5 (standalone engine), if we need a mobile app or complex offline support, we may need to add a proper frontend framework. This is a future concern, not a blocker.
