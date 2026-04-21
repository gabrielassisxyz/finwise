from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker, DeclarativeBase

from src.config import settings


engine = create_engine(
    settings.database_url,
    connect_args={"check_same_thread": False}
)

SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)


class Base(DeclarativeBase):
    pass


from src.models import settings as _settings
from src.models import chat_session as _chat_session
from src.models import message as _message
from src.models import pending_transaction as _pending_transaction
from src.models import payee_mapping as _payee_mapping
from src.models import sync_log as _sync_log


def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
