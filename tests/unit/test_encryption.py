"""Tests for EncryptionService using Fernet encryption."""

import pytest
from src.services.encryption import EncryptionService


class TestEncryptionService:
    """Test suite for EncryptionService."""

    def test_encrypt_decrypt_roundtrip(self):
        """Test that encrypted data can be decrypted back to original."""
        service = EncryptionService(key="test-key-at-least-32-chars-long!!")
        plaintext = "sensitive-data"

        ciphertext = service.encrypt(plaintext)
        decrypted = service.decrypt(ciphertext)

        assert decrypted == plaintext

    def test_encrypt_returns_none_for_none_input(self):
        """Test that encrypt returns None when input is None."""
        service = EncryptionService(key="test-key-at-least-32-chars-long!!")

        result = service.encrypt(None)

        assert result is None

    def test_decrypt_returns_none_for_none_input(self):
        """Test that decrypt returns None when input is None."""
        service = EncryptionService(key="test-key-at-least-32-chars-long!!")

        result = service.decrypt(None)

        assert result is None

    def test_key_too_short_raises_value_error(self):
        """Test that key shorter than 32 characters raises ValueError."""
        with pytest.raises(ValueError) as exc_info:
            EncryptionService(key="short-key")

        assert "key must be at least 32 characters" in str(exc_info.value)

    def test_key_exactly_32_chars_works(self):
        """Test that key with exactly 32 characters is accepted."""
        service = EncryptionService(key="12345678901234567890123456789012")

        ciphertext = service.encrypt("test")
        decrypted = service.decrypt(ciphertext)

        assert decrypted == "test"
