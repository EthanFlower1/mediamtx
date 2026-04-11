"""Schedule models."""

from __future__ import annotations

from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel

from kaivue.models.camera import RecordingMode


class ScheduleEntry(BaseModel):
    day_of_week: int  # ISO 8601: Monday=1..Sunday=7
    start_minute: int
    end_minute: int
    mode: RecordingMode = RecordingMode.UNSPECIFIED


class Schedule(BaseModel):
    id: str
    camera_id: str
    name: str = ""
    timezone: str = ""
    entries: List[ScheduleEntry] = []
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
