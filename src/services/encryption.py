"""Fernet encryption service for sensitive data."""

import hashlib
import base64
from cryptography.fernet import Fernet


class EncryptionService:
    """Service for encrypting and decrypting sensitive data using Fernet."""

    def __init__(self, key: str):
        """Initialize EncryptionService with a raw key.
        
        Args:
            key: Raw key string, must be at least 32 characters.
            
        Raises:
            ValueError: If key is shorter than 32 characters.
        """
        if len(key) < 32:
            raise ValueError("key must be at least 32 characters long")
        
        # Derive Fernet key using SHA256 + base64.urlsafe_b64encode
        key_hash = hashlib.sha256(key.encode()).digest()
        fernet_key = base64.urlsafe_b64encode(key_hash)
        self._fernet = Fernet(fernet_key)

    def encrypt(self, plaintext: str | None) -> str | None:
        """Encrypt plaintext string.
        
        Args:
            plaintext: String to encrypt.
            
        Returns:
            Encrypted string, or None if input is None.
        """
        if plaintext is None:
            return None
        
        return self._fernet.encrypt(plaintext.encode()).decode()

    def decrypt(self, ciphertext: str | None) -> str | None:
        """Decrypt ciphertext string.
        
        Args:
            ciphertext: String to decrypt.
            
        Returns:
            Decrypted string, or None if input is None.
        """
        if ciphertext is None:
            return None
        
        return self._fernet.decrypt(ciphertext.encode()).decode()
