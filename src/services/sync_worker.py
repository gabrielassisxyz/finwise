from typing import List

from sqlalchemy.orm import Session

from src.models.pending_transaction import PendingTransaction
from src.models.sync_log import SyncLog
from src.services.actual_budget import ActualBudgetClient


class SyncWorker:
    def __init__(self, db: Session, ab_client: ActualBudgetClient, token: str):
        self.db = db
        self.ab = ab_client
        self.token = token

    async def sync_transaction(self, tx: PendingTransaction) -> bool:
        try:
            ab_id = await self.ab.create_transaction(self.token, {
                "date": tx.txn_date.isoformat(),
                "payee": tx.payee,
                "amount": float(tx.amount),
                "category": tx.category,
                "notes": tx.notes or "",
            })

            tx.status = "synced"
            tx.actual_budget_id = ab_id
            tx.sync_error = None

            log = SyncLog(
                pending_transaction_id=tx.id,
                actual_budget_transaction_id=ab_id,
                sync_status="success",
            )
            self.db.add(log)
            self.db.commit()

            return True

        except Exception as e:
            tx.sync_error = str(e)
            self.db.commit()

            log = SyncLog(
                pending_transaction_id=tx.id,
                sync_status="failed",
                error_message=str(e),
            )
            self.db.add(log)
            self.db.commit()

            return False

    async def sync_batch(self, transactions: List[PendingTransaction]) -> dict:
        results = {"synced": 0, "failed": 0}
        for tx in transactions:
            success = await self.sync_transaction(tx)
            if success:
                results["synced"] += 1
            else:
                results["failed"] += 1
        return results
