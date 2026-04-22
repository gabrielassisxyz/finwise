from pydantic import BaseModel, Field
from typing import Optional


class SettingsBase(BaseModel):
    actual_budget_url: Optional[str] = None
    llm_provider: str = "openai"
    llm_model: str = "gpt-4o"
    auto_sync_threshold: float = Field(1.0, ge=0.0, le=1.0)
    default_currency: str = "USD"


class SettingsCreate(SettingsBase):
    llm_api_key: Optional[str] = None
    actual_budget_password: Optional[str] = None


class SettingsResponse(SettingsBase):
    id: int
    use_streaming: bool = True

    class Config:
        from_attributes = True


class SettingsUpdate(BaseModel):
    actual_budget_url: Optional[str] = None
    actual_budget_password: Optional[str] = None
    llm_provider: Optional[str] = None
    llm_api_key: Optional[str] = None
    llm_model: Optional[str] = None
    auto_sync_threshold: Optional[float] = Field(None, ge=0.0, le=1.0)
    default_currency: Optional[str] = None
