from sqlalchemy import Column, Integer, String, Numeric, DateTime
from sqlalchemy.sql import func

from src.database import Base


class Settings(Base):
    __tablename__ = "settings"

    id = Column(Integer, primary_key=True, index=True)
    actual_budget_url = Column(String, nullable=True)
    actual_budget_password_encrypted = Column(String, nullable=True)
    llm_provider = Column(String, nullable=True)
    llm_api_key_encrypted = Column(String, nullable=True)
    llm_model = Column(String, nullable=True)
    auto_sync_threshold = Column(Numeric(3, 2), nullable=True)
    use_streaming = Column(String, nullable=True)
    default_currency = Column(String, nullable=True)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    updated_at = Column(DateTime(timezone=True), onupdate=func.now())
