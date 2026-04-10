"""KaiVue VMS Python SDK.

Manage cameras, users, recordings, events, schedules, retention policies,
and integrations through the KaiVue public API.

Usage:
    from kaivue import KaiVueClient

    client = KaiVueClient("https://your-instance.kaivue.io", api_key="your-key")
    cameras = client.cameras.list()
"""

from kaivue.client import KaiVueClient
from kaivue.auth import APIKeyAuth, OAuthAuth
from kaivue.errors import (
    KaiVueError,
    AuthenticationError,
    NotFoundError,
    ValidationError,
    RateLimitError,
    ServerError,
)
from kaivue.models.camera import Camera, CameraState, RecordingMode, StreamProfile
from kaivue.models.user import User
from kaivue.models.recording import Recording
from kaivue.models.event import Event, EventKind, BoundingBox
from kaivue.models.schedule import Schedule, ScheduleEntry
from kaivue.models.retention import RetentionPolicy
from kaivue.models.integration import Integration, IntegrationKind

__version__ = "0.1.0"

__all__ = [
    "KaiVueClient",
    "APIKeyAuth",
    "OAuthAuth",
    "KaiVueError",
    "AuthenticationError",
    "NotFoundError",
    "ValidationError",
    "RateLimitError",
    "ServerError",
    "Camera",
    "CameraState",
    "RecordingMode",
    "StreamProfile",
    "User",
    "Recording",
    "Event",
    "EventKind",
    "BoundingBox",
    "Schedule",
    "ScheduleEntry",
    "RetentionPolicy",
    "Integration",
    "IntegrationKind",
]
