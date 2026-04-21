import asyncio
from typing import Optional, List, Dict, Any
from datetime import datetime

import httpx


class ActualBudgetClient:
    def __init__(self, base_url: str, password: str):
        self.base_url = base_url.rstrip("/")
        self.password = password
        self._client = httpx.AsyncClient(timeout=30.0)
        self._semaphore = asyncio.Semaphore(10)
        self._last_call_time: Optional[datetime] = None
        self._min_interval = 6.0

    async def _rate_limited_request(self, method: str, path: str, **kwargs) -> httpx.Response:
        async with self._semaphore:
            if self._last_call_time:
                elapsed = (datetime.utcnow() - self._last_call_time).total_seconds()
                if elapsed < self._min_interval:
                    await asyncio.sleep(self._min_interval - elapsed)

            url = f"{self.base_url}{path}"
            for attempt in range(3):
                response = await self._client.request(method, url, **kwargs)
                self._last_call_time = datetime.utcnow()

                if response.status_code == 429:
                    wait = (2 ** attempt) + (asyncio.get_event_loop().time() % 1)
                    await asyncio.sleep(wait)
                    continue

                response.raise_for_status()
                return response

            raise Exception("Actual Budget API rate limited after 3 retries")

    async def login(self) -> str:
        response = await self._rate_limited_request(
            "POST",
            "/login",
            json={"password": self.password},
        )
        data = response.json()
        return data.get("token") or data.get("data")

    async def get_categories(self, token: str) -> List[str]:
        response = await self._rate_limited_request(
            "GET",
            "/categories",
            headers={"Authorization": f"Bearer {token}"},
        )
        return [c["name"] for c in response.json().get("data", [])]

    async def create_transaction(self, token: str, transaction: Dict[str, Any]) -> str:
        response = await self._rate_limited_request(
            "POST",
            "/transactions",
            headers={"Authorization": f"Bearer {token}"},
            json=transaction,
        )
        data = response.json()
        return data.get("id") or data.get("data", {}).get("id")

    async def get_transactions(self, token: str, limit: int = 100) -> List[Dict[str, Any]]:
        response = await self._rate_limited_request(
            "GET",
            f"/transactions?limit={limit}",
            headers={"Authorization": f"Bearer {token}"},
        )
        return response.json().get("data", [])

    async def close(self):
        await self._client.aclose()
