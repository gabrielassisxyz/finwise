import hashlib


def compute_dedup_hash(date_str: str, payee: str, amount: float, upload_id: str) -> str:
    normalized = f"{date_str}|{payee.strip().lower()}|{amount:.2f}|{upload_id}"
    return hashlib.sha256(normalized.encode()).hexdigest()
