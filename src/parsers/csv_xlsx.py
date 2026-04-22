import pandas as pd
from typing import List, Dict, Any
from io import BytesIO


def parse_csv_xlsx(file_bytes: bytes, filename: str) -> List[Dict[str, Any]]:
    if filename.endswith(".csv"):
        df = pd.read_csv(BytesIO(file_bytes))
    elif filename.endswith((".xlsx", ".xls")):
        df = pd.read_excel(BytesIO(file_bytes))
    else:
        raise ValueError(f"Unsupported spreadsheet format: {filename}")

    if df.empty:
        raise ValueError("No rows found in spreadsheet")

    df.columns = [c.strip().lower() for c in df.columns]

    col_map = {}
    for col in df.columns:
        if col in ["date", "transaction date", "date posted"]:
            col_map["date"] = col
        elif col in ["payee", "description", "merchant", "name"]:
            col_map["payee"] = col
        elif col in ["amount", "transaction amount", "amount (usd)"]:
            col_map["amount"] = col
        elif col in ["category", "category name"]:
            col_map["category"] = col

    transactions = []
    for _, row in df.iterrows():
        tx = {
            "date": str(row.get(col_map.get("date", "date"), "")),
            "payee": str(row.get(col_map.get("payee", "payee"), "")),
            "amount": float(row.get(col_map.get("amount", "amount"), 0)),
            "category": str(row.get(col_map.get("category", "category"), "")) if "category" in col_map else None,
        }
        transactions.append(tx)

    return transactions
