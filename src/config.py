from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env")
    """Application settings loaded from environment variables."""

    # FinWise authentication
    finwise_secret_key: str
    finwise_password: str

    # LLM configuration
    llm_provider: str = "openai"
    llm_api_key: str
    llm_model: str = "gpt-4o-mini"
    llm_base_url: str | None = None

    # Actual Budget integration
    actual_budget_url: str
    actual_budget_password: str

    # Database
    database_url: str = "sqlite:///data/finwise.db"

    # Sync configuration
    auto_sync_threshold: float = 1.0
    streaming_max_transactions: int = 30


settings = Settings()
