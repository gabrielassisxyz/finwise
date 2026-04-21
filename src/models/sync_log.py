from sqlalchemy import Column, Integer, String, ForeignKey, DateTime, Text
from sqlalchemy.sql import func

from src.database import Base


class SyncLog(Base):
    __tablename__ = "sync_logs"

    id = Column(Integer, primary_key=True, index=True)
    pending_transaction_id = Column(Integer, ForeignKey("pending_transactions.id"), nullable=False)
    actual_budget_transaction_id = Column(String, nullable=True)
    sync_status = Column(String, nullable=False)
    error_message = Column(Text, nullable=True)
    synced_at = Column(DateTime(timezone=True), server_default=func.now())
