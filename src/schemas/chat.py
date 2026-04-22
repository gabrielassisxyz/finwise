from pydantic import BaseModel
from typing import Optional, List
from datetime import datetime


class ChatMessage(BaseModel):
    id: int
    role: str
    content: str
    attachments: Optional[str] = None
    created_at: datetime

    class Config:
        from_attributes = True


class ChatMessagesPaginated(BaseModel):
    messages: List[ChatMessage]
    has_more: bool
    total: int
