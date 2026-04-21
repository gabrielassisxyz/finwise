from fastapi import APIRouter, Request, Depends

from src.database import get_db
from sqlalchemy.orm import Session
from src.templates import templates
from src.models.settings import Settings as SettingsModel

router = APIRouter()


@router.get("/")
async def root(request: Request):
    return templates.TemplateResponse(request, "pages/chat.html")


@router.get("/chat")
async def chat_page(request: Request):
    return templates.TemplateResponse(request, "pages/chat.html")


@router.get("/history")
async def history_page(request: Request):
    return templates.TemplateResponse(request, "pages/history.html")


@router.get("/settings")
async def settings_page(request: Request, db: Session = Depends(get_db)):
    config = db.query(SettingsModel).first()
    return templates.TemplateResponse(
        request,
        "pages/settings.html",
        {"settings": config},
    )


@router.get("/setup")
async def setup_page(request: Request, db: Session = Depends(get_db)):
    config = db.query(SettingsModel).first()
    return templates.TemplateResponse(
        request,
        "pages/setup.html",
        {"settings": config, "completed": False},
    )
