import uuid
from typing import List, Dict, Any
from sqlalchemy.orm import Session

from src.models.pending_transaction import PendingTransaction
from src.models.chat_session import ChatSession
from src.utils.dedup import compute_dedup_hash
from src.services.bookkeeper import BookkeeperService


class UploadOrchestrator:
    def __init__(, db: Session, bookkeeper: BookkeeperService):
        self.db = db
        self.bookkeeper = bookkeeper

    def create_job(self, session_id: int, source: str) -> str:
        job_id = str(uuid.uuid4())
        session = self.db.query(ChatSession).filter_by(id=session_id).first()
        if session:
            session.active_job_id = job_id
            self.db.commit()
        return job_id

    def clear_job(self, session_id: int):
        session = self.db.query(ChatSession).filter_by(id=session_id).first()
        if session:
            session.active_job_id = None
            self.db.commit()

    def store_transactions(
        self,
        job_id: str,
        message_id: int,
        upload_id: str,
        transactions: List[Dict[str, Any]],
        source: str,
    ) -> List[PendingTransaction]:
        stored = []
        for tx in transactions:
            dedup = compute_dedup_hash(
                tx.get("date", ""),
                tx.get("payee", ""),
                tx.get("amount", 0.0),
                upload_id,
            )
            pt = PendingTransaction(
                message_id=message_id,
                job_id=job_id,
                txn_date=tx.get("date"),
                payee=tx.get("payee", ""),
                amount=tx.get("amount", 0.0),
                category=tx.get("category"),
                notes=tx.get("notes"),
                confidence=tx.get("confidence", 0.0),
                status="pending",
                source=source,
                raw_data=str(tx),
                dedup_hash=dedup,
                upload_id=upload_id,
            )
            self.db.add(pt)
            stored.append(pt)
        self.db.commit()
        return stored
