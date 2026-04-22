from .pages import router as pages_router
from .chat import router as chat_router
from .upload import router as upload_router
from .transactions import router as transactions_router
from .settings import router as settings_router
from .setup import router as setup_router

__all__ = [
    "pages_router",
    "chat_router",
    "upload_router",
    "transactions_router",
    "settings_router",
    "setup_router",
]
