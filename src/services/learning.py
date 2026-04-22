from typing import List, Dict, Any
import re

from sqlalchemy.orm import Session

from src.models.payee_mapping import PayeeMapping


def normalize_payee(name: str) -> str:
    return re.sub(r"[^\w\s]", "", name.lower().strip())


class LearningService:
    def __init__(self, db: Session):
        self.db = db

    def learn(self, payee: str, category: str | None):
        normalized = normalize_payee(payee)
        mapping = self.db.query(PayeeMapping).filter_by(payee=normalized).first()

        if mapping:
            mapping.frequency += 1
            if category:
                mapping.category = category
        else:
            mapping = PayeeMapping(
                payee=normalized,
                category=category,
                frequency=1,
            )
            self.db.add(mapping)

        self.db.commit()

    def get_top_mappings(self, n: int = 10) -> List[Dict[str, str]]:
        mappings = (
            self.db.query(PayeeMapping)
            .order_by(PayeeMapping.frequency.desc())
            .limit(n)
            .all()
        )
        return [{"payee": m.payee, "category": m.category} for m in mappings]

    def seed_from_transactions(self, transactions: List[Dict[str, Any]]):
        for tx in transactions:
            payee = tx.get("payee") or tx.get("payee_name")
            category = tx.get("category")
            if payee:
                self.learn(payee, category)
