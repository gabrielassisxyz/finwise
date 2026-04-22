from fastapi import APIRouter, Request, Depends, Query
from fastapi.responses import HTMLResponse, StreamingResponse
from sqlalchemy.orm import Session
from typing import Optional

from src.database import get_db
from src.models.message import Message
from src.models.chat_session import ChatSession
from src.schemas.chat import ChatMessagesPaginated
from src.templates import templates

router = APIRouter(prefix="/chat", tags=["chat"])


@router.get("/messages", response_model=ChatMessagesPaginated)
async def get_messages(
    session_id: int = Query(...),
    limit: int = Query(50, ge=1, le=200),
    before_id: Optional[int] = Query(None),
    db: Session = Depends(get_db),
):
    query = db.query(Message).filter(Message.session_id == session_id)
    if before_id:
        query = query.filter(Message.id < before_id)

    total = query.count()
    messages = query.order_by(Message.id.desc()).limit(limit).all()
    messages.reverse()

    return ChatMessagesPaginated(
        messages=messages,
        has_more=len(messages) == limit and total > limit,
        total=total,
    )


@router.post("/send")
async def send_message(
    request: Request,
    session_id: int = Query(...),
    content: str = Query(...),
    db: Session = Depends(get_db),
):
    session = db.query(ChatSession).filter_by(id=session_id).first()
    if not session:
        session = ChatSession(persona="bookkeeper")
        db.add(session)
        db.commit()
        db.refresh(session)

    msg = Message(session_id=session.id, role="user", content=content)
    db.add(msg)
    db.commit()
    db.refresh(msg)

    return templates.TemplateResponse(
        request,
        "partials/chat_message.html",
        {"message": msg},
    )


@router.get("/stream/{job_id}")
async def stream_job(
    job_id: str,
    db: Session = Depends(get_db),
):
    from src.services.stream_processor import StreamProcessor

    async def event_generator():
        async for event in StreamProcessor.stream_extraction(db, job_id):
            yield event
            await asyncio.sleep(0.1)

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
    )
