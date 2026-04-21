from sqlalchemy import Column, Integer, String, DateTime
from sqlalchemy.sql import func

from src.database import Base


class ChatSession(Base):
    __tablename__ = "chat_sessions"

    id = Column(Integer, primary_key=True, index=True)
    persona = Column(String, default="bookkeeper")
    active_job_id = Column(String, nullable=True, index=True)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    updated_at = Column(DateTime(timezone=True), onupdate=func.now())
