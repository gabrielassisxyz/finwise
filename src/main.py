from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles

from src.routers import (
    pages_router,
    chat_router,
    upload_router,
    transactions_router,
    settings_router,
    setup_router,
)


def create_app() -> FastAPI:
    app = FastAPI(title="FinWise")

    app.mount("/static", StaticFiles(directory="static"), name="static")

    @app.get("/health")
    def health():
        return {"status": "ok"}

    app.include_router(pages_router)
    app.include_router(chat_router)
    app.include_router(upload_router)
    app.include_router(transactions_router)
    app.include_router(settings_router)
    app.include_router(setup_router)

    return app


app = create_app()
