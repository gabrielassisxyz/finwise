from PIL import Image
import io


def prepare_image_for_llm(image_bytes: bytes, max_size: tuple = (2048, 2048)) -> bytes:
    img = Image.open(io.BytesIO(image_bytes))
    img.thumbnail(max_size)
    buffer = io.BytesIO()
    img.convert("RGB").save(buffer, format="JPEG", quality=85)
    return buffer.getvalue()
