from typing import List, Dict, Any


def parse_text(file_bytes: bytes) -> List[Dict[str, Any]]:
    text = file_bytes.decode("utf-8", errors="replace")
    return [{"text": text}]
