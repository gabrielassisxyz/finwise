from pydantic import BaseModel
from typing import Optional


class UploadInitResponse(BaseModel):
    job_id: str
    stream_url: str


class SSEEvent(BaseModel):
    event: str
    data: dict
    id: Optional[str] = None
