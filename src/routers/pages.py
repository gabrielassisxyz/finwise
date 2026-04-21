from fastapi import APIRouter, Request, Depends
from fastapi.templating import Jinja2Templates

from src.database import get_db
from sqlalchemy.orm import Session

router = APIRouter()
templates = Jinja2Templates(directory="src/templates")


@router.get("/")
async def root(request: Request):
    return templates.TemplateResponse("pages/chat.html", {"request": request})


@router.get("/chat")
async def chat_page(request: Request):
    return templates.TemplateResponse("pages/chat.html", {"request": request})


@router.get("/history")
async def history_page(request: Request):
    return templates.TemplateResponse("pages/history.html", {"request": request})


@router.get("/settings")
async def settings_page(request: Request):
    return templates.TemplateResponse("pages/settings.html", {"request": request})


@router.get("/setup")
async def setup_page(request: Request):
    return templates.TemplateResponse("pages/setup.html", {"request": request})
