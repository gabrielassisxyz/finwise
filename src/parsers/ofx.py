from ofxparse import OfxParser
from typing import List, Dict, Any
from io import BytesIO


def parse_ofx(file_bytes: bytes) -> List[Dict[str, Any]]:
    ofx = OfxParser.parse(BytesIO(file_bytes))
    transactions = []

    for account in ofx.accounts:
        for txn in account.statement.transactions:
            transactions.append({
                "date": txn.date.strftime("%Y-%m-%d") if txn.date else "",
                "payee": txn.payee or "",
                "amount": float(txn.amount) if txn.amount else 0.0,
                "category": None,
            })

    return transactions
