import base64
from typing import List, Dict, Any

import litellm


class LLMClient:
    def __init__(self, provider: str, api_key: str, model: str, base_url: str | None = None):
        self.provider = provider
        self.model = model
        self.api_key = api_key
        self.base_url = base_url

        litellm.api_key = api_key
        if base_url:
            litellm.api_base = base_url

    def _build_messages(self, prompt: str, system: str | None = None) -> List[Dict[str, str]]:
        messages = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})
        return messages

    async def chat(
        self,
        prompt: str,
        system: str | None = None,
        json_mode: bool = False,
        temperature: float = 0.1,
    ) -> str:
        messages = self._build_messages(prompt, system)
        response_format = {"type": "json_object"} if json_mode else None

        response = await litellm.acompletion(
            model=self.model,
            messages=messages,
            response_format=response_format,
            temperature=temperature,
        )
        return response.choices[0].message.content

    async def chat_with_image(
        self,
        prompt: str,
        image_bytes: bytes,
        system: str | None = None,
        json_mode: bool = False,
    ) -> str:
        b64_image = base64.b64encode(image_bytes).decode("utf-8")

        content = [
            {"type": "text", "text": prompt},
            {
                "type": "image_url",
                "image_url": {"url": f"data:image/jpeg;base64,{b64_image}"},
            },
        ]

        messages = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": content})

        response_format = {"type": "json_object"} if json_mode else None

        response = await litellm.acompletion(
            model=self.model,
            messages=messages,
            response_format=response_format,
            temperature=0.1,
        )
        return response.choices[0].message.content
