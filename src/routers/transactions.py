from fastapi import APIRouter, Request, Depends, Form
from sqlalchemy.orm import Session
from decimal import Decimal

from src.database import get_db
from src.models.pending_transaction import PendingTransaction
from src.services.learning import LearningService
from src.templates import templates

router = APIRouter(prefix="/transactions", tags=["transactions"])


@router.post("/{tx_id}/edit")
async def edit_transaction(
    request: Request,
    tx_id: int,
    payee: str = Form(...),
    amount: str = Form(...),
    category: str = Form(None),
    notes: str = Form(None),
    db: Session = Depends(get_db),
):
    tx = db.query(PendingTransaction).filter_by(id=tx_id).first()
    if not tx:
        return {"error": "Transaction not found"}

    old_category = tx.category
    tx.payee = payee
    tx.amount = Decimal(amount)
    tx.category = category
    tx.notes = notes
    db.commit()

    if category and category != old_category:
        learning = LearningService(db)
        learning.learn(payee, category)

    return templates.TemplateResponse(
        request,
        "partials/transaction_card.html",
        {"tx": tx},
    )


@router.post("/{tx_id}/confirm")
async def confirm_transaction(
    request: Request,
    tx_id: int,
    db: Session = Depends(get_db),
):
    tx = db.query(PendingTransaction).filter_by(id=tx_id).first()
    if not tx:
        return {"error": "Transaction not found"}

    tx.status = "synced"
    db.commit()

    return templates.TemplateResponse(
        request,
        "partials/transaction_card.html",
        {"tx": tx},
    )


@router.post("/{tx_id}/reject")
async def reject_transaction(
    request: Request,
    tx_id: int,
    db: Session = Depends(get_db),
):
    tx = db.query(PendingTransaction).filter_by(id=tx_id).first()
    if not tx:
        return {"error": "Transaction not found"}

    tx.status = "rejected"
    db.commit()

    return templates.TemplateResponse(
        request,
        "partials/transaction_card.html",
        {"tx": tx},
    )


@router.post("/bulk-confirm")
async def bulk_confirm(
    request: Request,
    job_id: str = Form(...),
    db: Session = Depends(get_db),
):
    txs = (
        db.query(PendingTransaction)
        .filter_by(job_id=job_id, status="pending")
        .all()
    )

    for tx in txs:
        tx.status = "synced"

    db.commit()

    return templates.TemplateResponse(
        request,
        "partials/bulk_confirm_result.html",
        {"count": len(txs)},
    )
