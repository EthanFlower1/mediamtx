"""Retention policy models."""

from __future__ import annotations

from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel


class RetentionPolicy(BaseModel):
    id: str
    name: str = ""
    description: str = ""
    retention_days: int = 0
    max_bytes: int = 0
    event_retention_days: int = 0
    camera_ids: List[str] = []
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
