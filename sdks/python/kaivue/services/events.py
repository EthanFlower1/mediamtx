"""Event service client."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Dict, Generator, List, Optional

from kaivue.models.event import Event, EventKind
from kaivue.services.base import BaseService


class EventService(BaseService):
    """Operations for AI/motion events."""

    def get(self, event_id: str) -> Event:
        resp = self._get(f"/v1/events/{event_id}")
        return self._parse(Event, resp["event"])

    def list(
        self,
        *,
        camera_id: Optional[str] = None,
        kinds: Optional[List[EventKind]] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        min_confidence: Optional[float] = None,
        query: Optional[str] = None,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[Event]:
        params: Dict[str, Any] = {"page_size": page_size}
        if camera_id:
            params["camera_id"] = camera_id
        if kinds:
            params["kinds"] = ",".join(k.value for k in kinds)
        if start_time:
            params["start_time"] = start_time.isoformat()
        if end_time:
            params["end_time"] = end_time.isoformat()
        if min_confidence is not None:
            params["min_confidence"] = min_confidence
        if query:
            params["query"] = query
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/events", **params)
        return self._parse_list(Event, resp.get("events", []))

    def list_all(
        self,
        *,
        camera_id: Optional[str] = None,
        kinds: Optional[List[EventKind]] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        min_confidence: Optional[float] = None,
        query: Optional[str] = None,
        page_size: int = 50,
    ) -> Generator[Event, None, None]:
        params: Dict[str, Any] = {}
        if camera_id:
            params["camera_id"] = camera_id
        if kinds:
            params["kinds"] = ",".join(k.value for k in kinds)
        if start_time:
            params["start_time"] = start_time.isoformat()
        if end_time:
            params["end_time"] = end_time.isoformat()
        if min_confidence is not None:
            params["min_confidence"] = min_confidence
        if query:
            params["query"] = query
        yield from self._paginate(Event, "/v1/events", "events", page_size=page_size, **params)

    def acknowledge(self, event_id: str, *, note: str = "") -> Event:
        body: Dict[str, Any] = {"id": event_id}
        if note:
            body["note"] = note
        resp = self._post(f"/v1/events/{event_id}/acknowledge", body)
        return self._parse(Event, resp["event"])
