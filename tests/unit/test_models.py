"""Unit tests for SQLAlchemy models."""

import pytest
from sqlalchemy import inspect

from src.models import (
    Settings,
    ChatSession,
    Message,
    PendingTransaction,
    PayeeMapping,
    SyncLog,
)


class TestSettings:
    def test_has_required_columns(self):
        mapper = inspect(Settings)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "actual_budget_url" in columns
        assert "actual_budget_password_encrypted" in columns
        assert "llm_provider" in columns
        assert "llm_api_key_encrypted" in columns
        assert "llm_model" in columns
        assert "auto_sync_threshold" in columns
        assert "use_streaming" in columns
        assert "default_currency" in columns
        assert "created_at" in columns
        assert "updated_at" in columns


class TestChatSession:
    def test_has_required_columns(self):
        mapper = inspect(ChatSession)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "persona" in columns
        assert "active_job_id" in columns


class TestMessage:
    def test_has_required_columns(self):
        mapper = inspect(Message)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "session_id" in columns
        assert "role" in columns
        assert "content" in columns
        assert "attachments" in columns
        assert "created_at" in columns


class TestPendingTransaction:
    def test_has_required_columns(self):
        mapper = inspect(PendingTransaction)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "message_id" in columns
        assert "job_id" in columns
        assert "txn_date" in columns
        assert "payee" in columns
        assert "amount" in columns
        assert "category" in columns
        assert "notes" in columns
        assert "confidence" in columns
        assert "status" in columns
        assert "source" in columns
        assert "raw_data" in columns
        assert "dedup_hash" in columns
        assert "actual_budget_id" in columns
        assert "sync_error" in columns
        assert "upload_id" in columns
        assert "created_at" in columns
        assert "updated_at" in columns

    def test_has_composite_indexes(self):
        table = PendingTransaction.__table__
        indexes = [idx.name for idx in table.indexes]
        assert "idx_pending_tx_job_created" in indexes
        assert "idx_pending_tx_confidence" in indexes


class TestPayeeMapping:
    def test_has_required_columns(self):
        mapper = inspect(PayeeMapping)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "payee" in columns
        assert "category" in columns
        assert "frequency" in columns
        assert "last_used" in columns
        assert "created_at" in columns
        assert "updated_at" in columns


class TestSyncLog:
    def test_has_required_columns(self):
        mapper = inspect(SyncLog)
        columns = [c.key for c in mapper.columns]
        assert "id" in columns
        assert "pending_transaction_id" in columns
        assert "actual_budget_transaction_id" in columns
        assert "sync_status" in columns
        assert "error_message" in columns
        assert "synced_at" in columns
