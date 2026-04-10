"""Recording models."""

from __future__ import annotations

from datetime import datetime
from typing import Optional

from pydantic import BaseModel


class Recording(BaseModel):
    id: str
    camera_id: str
    recorder_id: str = ""
    start_time: Optional[datetime] = None
    end_time: Optional[datetime] = None
    size_bytes: int = 0
    codec: str = ""
    has_audio: bool = False
    is_event_clip: bool = False
    storage_tier: str = ""
