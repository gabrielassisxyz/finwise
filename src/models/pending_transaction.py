from sqlalchemy import Column, Integer, String, ForeignKey, DateTime, Text, Numeric, Date, Index
from sqlalchemy.sql import func

from src.database import Base


class PendingTransaction(Base):
    __tablename__ = "pending_transactions"

    id = Column(Integer, primary_key=True, index=True)
    message_id = Column(Integer, ForeignKey("messages.id"), index=True)
    job_id = Column(String, index=True)
    txn_date = Column(Date, nullable=False)
    payee = Column(String, nullable=False)
    amount = Column(Numeric(10, 2), nullable=False)
    category = Column(String, nullable=True)
    notes = Column(Text, nullable=True)
    confidence = Column(Numeric(3, 2), nullable=False)
    status = Column(String, default="pending", index=True)
    source = Column(String, nullable=False)
    raw_data = Column(Text, nullable=True)
    dedup_hash = Column(String(64), index=True)
    actual_budget_id = Column(String, nullable=True, index=True)
    sync_error = Column(Text, nullable=True)
    upload_id = Column(String, index=True)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    updated_at = Column(DateTime(timezone=True), onupdate=func.now())

    __table_args__ = (
        Index("idx_pending_tx_job_created", "job_id", "created_at"),
        Index("idx_pending_tx_confidence", "confidence", "status", "created_at"),
    )
