from fastapi import APIRouter, Request, Form, Depends
from fastapi.templating import Jinja2Templates
from sqlalchemy.orm import Session

from src.database import get_db
from src.models.settings import Settings as SettingsModel
from src.services.encryption import EncryptionService
from src.config import settings

router = APIRouter(prefix="/setup", tags=["setup"])
templates = Jinja2Templates(directory="src/templates")


@router.get("/")
async def setup_page(request: Request, db: Session = Depends(get_db)):
    config = db.query(SettingsModel).first()
    if config and config.llm_api_key_encrypted:
        return {"status": "already_configured"}

    return templates.TemplateResponse("pages/setup.html", {"request": request})


@router.post("/")
async def setup_save(
    request: Request,
    llm_provider: str = Form(...),
    llm_api_key: str = Form(...),
    llm_model: str = Form(...),
    actual_budget_url: str = Form(...),
    actual_budget_password: str = Form(...),
    db: Session = Depends(get_db),
):
    enc = EncryptionService(settings.finwise_secret_key)

    config = SettingsModel(
        llm_provider=llm_provider,
        llm_api_key_encrypted=enc.encrypt(llm_api_key),
        llm_model=llm_model,
        actual_budget_url=actual_budget_url,
        actual_budget_password_encrypted=enc.encrypt(actual_budget_password),
    )
    db.add(config)
    db.commit()

    return templates.TemplateResponse(
        "pages/setup.html",
        {"request": request, "completed": True},
    )
