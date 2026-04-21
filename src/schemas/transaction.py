from pydantic import BaseModel
from datetime import date
from typing import Optional
from decimal import Decimal


class TransactionBase(BaseModel):
    txn_date: date
    payee: str
    amount: Decimal
    category: Optional[str] = None
    notes: Optional[str] = None
    confidence: Decimal
    status: str = "pending"
    source: str


class TransactionCreate(TransactionBase):
    job_id: str
    dedup_hash: str
    upload_id: str
    raw_data: Optional[str] = None


class TransactionResponse(TransactionBase):
    id: int
    job_id: str
    actual_budget_id: Optional[str] = None
    sync_error: Optional[str] = None
    created_at: str

    class Config:
        from_attributes = True


class TransactionEdit(BaseModel):
    txn_date: Optional[date] = None
    payee: Optional[str] = None
    amount: Optional[Decimal] = None
    category: Optional[str] = None
    notes: Optional[str] = None
