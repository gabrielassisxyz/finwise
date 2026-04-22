import json
from typing import List, Dict, Any

from src.services.llm_client import LLMClient


EXTRACTION_SCHEMA = {
    "type": "object",
    "properties": {
        "narration": {"type": "string"},
        "transactions": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "date": {"type": "string"},
                    "payee": {"type": "string"},
                    "amount": {"type": "number"},
                    "category": {"type": "string"},
                    "notes": {"type": "string"},
                    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
                },
                "required": ["date", "payee", "amount", "confidence"],
            },
        },
    },
    "required": ["narration", "transactions"],
}


class BookkeeperService:
    """Service for extracting transactions from text/images using LLM."""

    def __init__(
        self,
        llm_client: LLMClient,
        categories: List[str] | None = None,
        payee_mappings: List[Dict[str, str]] | None = None,
    ):
        self.llm = llm_client
        self.categories = categories or []
        self.payee_mappings = payee_mappings or []

    def _build_system_prompt(self) -> str:
        categories_str = ", ".join(self.categories) if self.categories else "any relevant category"
        top_payees = self.payee_mappings[:10]

        payee_context = ""
        if top_payees:
            payee_lines = [f"- {m['payee']} -> {m['category']}" for m in top_payees]
            payee_context = "\nUser's known preferences:\n" + "\n".join(payee_lines)

        return (
            "You are the Bookkeeper. Extract all visible transactions from the input.\n"
            f"Available categories: {categories_str}{payee_context}\n\n"
            "Return JSON matching this schema:\n"
            '{"narration": "friendly summary", "transactions": [{"date": "YYYY-MM-DD", "payee": "string", "amount": -12.50, "category": "string", "notes": "string", "confidence": 0.95}]}'
        )

    async def extract_from_text(self, text: str) -> Dict[str, Any]:
        system = self._build_system_prompt()
        response = await self.llm.chat(
            prompt=text,
            system=system,
            json_mode=True,
        )
        return json.loads(response)

    async def extract_from_image(self, image_bytes: bytes) -> Dict[str, Any]:
        system = self._build_system_prompt()
        prompt = (
            "Extract all visible transactions from this image. "
            "Return JSON with 'narration' and 'transactions' array."
        )
        response = await self.llm.chat_with_image(
            prompt=prompt,
            image_bytes=image_bytes,
            system=system,
            json_mode=True,
        )
        return json.loads(response)
