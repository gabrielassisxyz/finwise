# ADR 002: LLM Integration — Provider-Agnostic Wrapper

**Status:** Accepted  
**Date:** 2025-04-21  
**Deciders:** Project owner (Gabriel) + AI assistant

---

## Context

FinWise requires an LLM for parsing transactions from natural language, images, and documents. The app must be fully self-hosted and open source. Users should not be locked into a single LLM provider.

## Constraints

- Must support user-provided API keys (no centralized API)
- Must work with popular providers (OpenAI, Anthropic)
- Must support local/self-hosted models (Ollama) for privacy
- Should be easy to add new providers
- Must handle both text and vision (image) inputs

## Alternatives Considered

### Alternative A: Hardcode OpenAI Only

**Description:** Direct integration with OpenAI's Python SDK.

**Pros:**
- Simplest implementation
- Best documentation and reliability

**Cons:**
- Locks users into OpenAI
- No privacy option (local models)
- No flexibility for users who prefer Anthropic or local models

**Rejected because:** Violates the open source, self-hosted, and user-choice principles.

### Alternative B: LangChain

**Description:** Use LangChain's abstraction layer for LLM integration.

**Pros:**
- Supports many providers out of the box
- Rich ecosystem of tools and integrations

**Cons:**
- Heavy dependency with many sub-dependencies
- Abstraction leaks and complexity
- Overkill for simple chat + JSON parsing use case
- Can be brittle with version changes

**Rejected because:** Adds significant complexity and dependency overhead for a simple use case. We don't need chains, agents, or vector stores.

### Alternative C: LiteLLM

**Description:** Use LiteLLM proxy/library for unified LLM access.

**Pros:**
- Drop-in replacement for OpenAI SDK
- Supports 100+ providers
- Active development

**Cons:**
- Additional dependency
- Some providers may have edge case issues
- More moving parts

**Considered but rejected because:** While LiteLLM is excellent, for a simple app with 3-4 providers, a thin custom wrapper is sufficient and avoids an extra dependency.

### Alternative D: Custom Provider-Agnostic Wrapper

**Description:** Build a thin abstraction around HTTP clients for each supported provider.

**Pros:**
- Full control over implementation
- No unnecessary dependencies
- Easy to understand and modify
- Direct mapping to each provider's features

**Cons:**
- Must implement each provider manually
- Small maintenance burden when APIs change

**Accepted because:** For 3-4 providers (OpenAI, Anthropic, Ollama, OpenAI-compatible), the implementation is ~200 lines. The tradeoff favors simplicity and control over using a heavy abstraction library.

## Decision

Build a **custom provider-agnostic LLM wrapper** with explicit support for OpenAI, Anthropic, Ollama, and generic OpenAI-compatible APIs.

## Tradeoffs

- **Maintenance vs control:** We accept a small maintenance burden for full control and zero unnecessary dependencies.
- **Flexibility vs simplicity:** The wrapper is intentionally thin — it doesn't abstract away provider-specific features (like Anthropic's system prompts), but provides a common interface for chat and vision.

## Implementation Approach

```python
class LLMClient:
    def __init__(self, provider: str, api_key: str, model: str, base_url: str = None):
        self.provider = provider
        self.model = model
        self.client = self._create_client(api_key, base_url)
    
    def chat(self, messages: list, json_mode: bool = False) -> str:
        # Route to provider-specific implementation
        ...
    
    def chat_with_image(self, messages: list, image_bytes: bytes) -> str:
        # Base64 encode image, send with vision model
        ...
```

- OpenAI: Use `openai` Python SDK
- Anthropic: Use `anthropic` Python SDK
- Ollama: Use raw HTTP to local Ollama API (OpenAI-compatible)
- Generic: Allow any OpenAI-compatible API via `base_url`

## Consequences

- **Positive:** Users can choose their provider, including fully local models. No vendor lock-in.
- **Negative:** Adding a new exotic provider requires code changes. However, the OpenAI-compatible route covers most cases.
