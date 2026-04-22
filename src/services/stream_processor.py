import asyncio
import json
from typing import AsyncGenerator, Dict, Any

from sqlalchemy.orm import Session

from src.models.pending_transaction import PendingTransaction


class StreamProcessor:
    @staticmethod
    def format_sse(event: str, data: Dict[str, Any], event_id: str | None = None) -> str:
        lines = [f"event: {event}"]
        if event_id:
            lines.append(f"id: {event_id}")
        lines.append(f"data: {json.dumps(data)}")
        lines.append("")
        return "\n".join(lines)

    @staticmethod
    async def stream_extraction(
        db: Session,
        job_id: str,
    ) -> AsyncGenerator[str, None]:
        transactions = (
            db.query(PendingTransaction)
            .filter_by(job_id=job_id)
            .order_by(PendingTransaction.created_at)
            .all()
        )

        if not transactions:
            yield StreamProcessor.format_sse(
                "complete",
                {"total": 0, "auto_synced": 0, "needs_review": 0},
                event_id=f"{job_id}-complete",
            )
            return

        for i, tx in enumerate(transactions):
            event_id = f"{job_id}-{tx.id}"
            data = {
                "id": tx.id,
                "date": tx.txn_date.isoformat() if tx.txn_date else None,
                "payee": tx.payee,
                "amount": float(tx.amount),
                "category": tx.category,
                "confidence": float(tx.confidence),
                "status": tx.status,
            }
            yield StreamProcessor.format_sse("transaction", data, event_id=event_id)
            await asyncio.sleep(0.05)

        synced = sum(1 for t in transactions if t.status == "synced")
        review = sum(1 for t in transactions if t.status == "needs_review")

        yield StreamProcessor.format_sse(
            "complete",
            {"total": len(transactions), "auto_synced": synced, "needs_review": review},
            event_id=f"{job_id}-complete",
        )

    @staticmethod
    def heartbeat() -> str:
        return StreamProcessor.format_sse("heartbeat", {"timestamp": asyncio.get_event_loop().time()})
