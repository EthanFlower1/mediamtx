"""Camera models."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import List, Optional

from pydantic import BaseModel


class CameraState(str, Enum):
    UNSPECIFIED = "CAMERA_STATE_UNSPECIFIED"
    PROVISIONING = "CAMERA_STATE_PROVISIONING"
    ONLINE = "CAMERA_STATE_ONLINE"
    OFFLINE = "CAMERA_STATE_OFFLINE"
    DISABLED = "CAMERA_STATE_DISABLED"
    ERROR = "CAMERA_STATE_ERROR"


class RecordingMode(str, Enum):
    UNSPECIFIED = "RECORDING_MODE_UNSPECIFIED"
    CONTINUOUS = "RECORDING_MODE_CONTINUOUS"
    MOTION = "RECORDING_MODE_MOTION"
    SCHEDULE = "RECORDING_MODE_SCHEDULE"
    EVENT = "RECORDING_MODE_EVENT"
    OFF = "RECORDING_MODE_OFF"


class StreamProfile(BaseModel):
    name: str
    codec: str
    width: int = 0
    height: int = 0
    bitrate_kbps: int = 0
    framerate: int = 0


class Camera(BaseModel):
    id: str
    name: str
    description: str = ""
    manufacturer: str = ""
    model: str = ""
    firmware_version: str = ""
    mac_address: str = ""
    ip_address: str = ""
    state: CameraState = CameraState.UNSPECIFIED
    recording_mode: RecordingMode = RecordingMode.UNSPECIFIED
    profiles: List[StreamProfile] = []
    labels: List[str] = []
    recorder_id: str = ""
    state_reported_at: Optional[datetime] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    audio_enabled: bool = False
    motion_sensitivity: int = 0
