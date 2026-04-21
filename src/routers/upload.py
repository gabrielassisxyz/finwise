import asyncio
import uuid

from fastapi import APIRouter, Request, UploadFile, File, Depends, Form
from fastapi.templating import Jinja2Templates
from sqlalchemy.orm import Session

from src.database import get_db
from src.models.chat_session import ChatSession
from src.models.message import Message
from src.services.bookkeeper import BookkeeperService
from src.services.llm_client import LLMClient
from src.services.upload_orchestrator import UploadOrchestrator
from src.parsers.image import prepare_image_for_llm
from src.config import settings

router = APIRouter(prefix="/chat", tags=["upload"])
templates = Jinja2Templates(directory="src/templates")


@router.post("/upload")
async def upload_file(
    request: Request,
    file: UploadFile = File(...),
    session_id: int = Form(1),
    db: Session = Depends(get_db),
):
    contents = await file.read()
    upload_id = str(uuid.uuid4())

    session = db.query(ChatSession).filter_by(id=session_id).first()
    if not session:
        session = ChatSession(persona="bookkeeper")
        db.add(session)
        db.commit()
        db.refresh(session)

    llm = LLMClient(
        provider=settings.llm_provider,
        api_key=settings.llm_api_key,
        model=settings.llm_model,
        base_url=settings.llm_base_url,
    )
    bookkeeper = BookkeeperService(llm)
    orchestrator = UploadOrchestrator(db, bookkeeper)

    job_id = orchestrator.create_job(session.id, "screenshot")

    msg = Message(
        session_id=session.id,
        role="user",
        content=f"Uploaded: {file.filename}",
        attachments=str({"filename": file.filename, "type": file.content_type}),
    )
    db.add(msg)
    db.commit()
    db.refresh(msg)

    image_bytes = prepare_image_for_llm(contents)
    result = await bookkeeper.extract_from_image(image_bytes)

    transactions = result.get("transactions", [])
    narration = result.get("narration", "")

    if transactions:
        orchestrator.store_transactions(
            job_id=job_id,
            message_id=msg.id,
            upload_id=upload_id,
            transactions=transactions,
            source="screenshot",
        )

    return templates.TemplateResponse(
        "partials/upload_result.html",
        {
            "request": request,
            "job_id": job_id,
            "narration": narration,
            "transactions": transactions,
        },
    )
