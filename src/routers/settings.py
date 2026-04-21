from fastapi import APIRouter, Request, Form, Depends
from fastapi.templating import Jinja2Templates
from sqlalchemy.orm import Session

from src.database import get_db
from src.models.settings import Settings as SettingsModel
from src.services.encryption import EncryptionService
from src.config import settings

router = APIRouter(prefix="/settings", tags=["settings"])
templates = Jinja2Templates(directory="src/templates")


@router.get("/")
async def get_settings(request: Request, db: Session = Depends(get_db)):
    config = db.query(SettingsModel).first()
    if not config:
        config = SettingsModel()
        db.add(config)
        db.commit()
        db.refresh(config)

    return templates.TemplateResponse(
        "pages/settings.html",
        {"request": request, "settings": config},
    )


@router.post("/")
async def save_settings(
    request: Request,
    llm_provider: str = Form(...),
    llm_api_key: str = Form(...),
    llm_model: str = Form(...),
    actual_budget_url: str = Form(...),
    actual_budget_password: str = Form(...),
    auto_sync_threshold: float = Form(1.0),
    db: Session = Depends(get_db),
):
    enc = EncryptionService(settings.finwise_secret_key)

    config = db.query(SettingsModel).first()
    if not config:
        config = SettingsModel()
        db.add(config)

    config.llm_provider = llm_provider
    config.llm_api_key_encrypted = enc.encrypt(llm_api_key)
    config.llm_model = llm_model
    config.actual_budget_url = actual_budget_url
    config.actual_budget_password_encrypted = enc.encrypt(actual_budget_password)
    config.auto_sync_threshold = auto_sync_threshold

    db.commit()
    db.refresh(config)

    return templates.TemplateResponse(
        "pages/settings.html",
        {"request": request, "settings": config, "saved": True},
    )
