"""Event models."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Dict, Optional

from pydantic import BaseModel


class EventKind(str, Enum):
    UNSPECIFIED = "EVENT_KIND_UNSPECIFIED"
    MOTION = "EVENT_KIND_MOTION"
    PERSON = "EVENT_KIND_PERSON"
    VEHICLE = "EVENT_KIND_VEHICLE"
    FACE = "EVENT_KIND_FACE"
    LICENSE_PLATE = "EVENT_KIND_LICENSE_PLATE"
    AUDIO_ALARM = "EVENT_KIND_AUDIO_ALARM"
    LINE_CROSSING = "EVENT_KIND_LINE_CROSSING"
    LOITERING = "EVENT_KIND_LOITERING"
    TAMPER = "EVENT_KIND_TAMPER"
    CUSTOM = "EVENT_KIND_CUSTOM"


class BoundingBox(BaseModel):
    x: float = 0.0
    y: float = 0.0
    width: float = 0.0
    height: float = 0.0


class Event(BaseModel):
    id: str
    camera_id: str
    kind: EventKind = EventKind.UNSPECIFIED
    kind_label: str = ""
    observed_at: Optional[datetime] = None
    confidence: float = 0.0
    bbox: Optional[BoundingBox] = None
    track_id: str = ""
    thumbnail_url: str = ""
    attributes: Dict[str, str] = {}
