"""Integration models."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Dict, List, Optional

from pydantic import BaseModel

from kaivue.models.event import EventKind


class IntegrationKind(str, Enum):
    UNSPECIFIED = "INTEGRATION_KIND_UNSPECIFIED"
    WEBHOOK = "INTEGRATION_KIND_WEBHOOK"
    MQTT = "INTEGRATION_KIND_MQTT"
    SYSLOG = "INTEGRATION_KIND_SYSLOG"
    CUSTOM = "INTEGRATION_KIND_CUSTOM"


class Integration(BaseModel):
    id: str
    name: str = ""
    kind: IntegrationKind = IntegrationKind.UNSPECIFIED
    enabled: bool = True
    config: Dict[str, str] = {}
    subscribed_events: List[EventKind] = []
    camera_ids: List[str] = []
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
