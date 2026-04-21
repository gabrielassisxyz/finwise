from sqlalchemy import Column, Integer, String, DateTime, UniqueConstraint
from sqlalchemy.sql import func

from src.database import Base


class PayeeMapping(Base):
    __tablename__ = "payee_mappings"

    id = Column(Integer, primary_key=True, index=True)
    payee = Column(String, nullable=False, unique=True, index=True)
    category = Column(String, nullable=False)
    frequency = Column(Integer, default=1)
    last_used = Column(DateTime(timezone=True), nullable=True)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    updated_at = Column(DateTime(timezone=True), onupdate=func.now())

    __table_args__ = (
        UniqueConstraint("payee", name="uq_payee_mappings_payee"),
    )
