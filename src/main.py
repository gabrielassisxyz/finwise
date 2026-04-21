from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles

from src.config import settings


def create_app() -> FastAPI:
    app = FastAPI(title="FinWise")

    app.mount("/static", StaticFiles(directory="static"), name="static")

    @app.get("/health")
    def health():
        return {"status": "ok"}

    return app


app = create_app()
