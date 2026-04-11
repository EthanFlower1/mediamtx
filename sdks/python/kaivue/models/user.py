"""User models."""

from __future__ import annotations

from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel


class User(BaseModel):
    id: str
    username: str
    email: str = ""
    display_name: str = ""
    groups: List[str] = []
    disabled: bool = False
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    last_login_at: Optional[datetime] = None
