"""Recording service client."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Dict, Generator, List, Optional

from kaivue.models.recording import Recording
from kaivue.services.base import BaseService


class RecordingService(BaseService):
    """Operations for recordings."""

    def get(self, recording_id: str) -> Recording:
        resp = self._get(f"/v1/recordings/{recording_id}")
        return self._parse(Recording, resp["recording"])

    def list(
        self,
        *,
        camera_id: Optional[str] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        event_clips_only: bool = False,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[Recording]:
        params: Dict[str, Any] = {"page_size": page_size}
        if camera_id:
            params["camera_id"] = camera_id
        if start_time:
            params["start_time"] = start_time.isoformat()
        if end_time:
            params["end_time"] = end_time.isoformat()
        if event_clips_only:
            params["event_clips_only"] = True
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/recordings", **params)
        return self._parse_list(Recording, resp.get("recordings", []))

    def list_all(
        self,
        *,
        camera_id: Optional[str] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        event_clips_only: bool = False,
        page_size: int = 50,
    ) -> Generator[Recording, None, None]:
        params: Dict[str, Any] = {}
        if camera_id:
            params["camera_id"] = camera_id
        if start_time:
            params["start_time"] = start_time.isoformat()
        if end_time:
            params["end_time"] = end_time.isoformat()
        if event_clips_only:
            params["event_clips_only"] = True
        yield from self._paginate(
            Recording, "/v1/recordings", "recordings", page_size=page_size, **params
        )

    def delete(self, recording_id: str) -> None:
        self._delete(f"/v1/recordings/{recording_id}")

    def export(
        self,
        camera_id: str,
        start_time: datetime,
        end_time: datetime,
        *,
        format: str = "mp4",
    ) -> Dict[str, Any]:
        """Export a recording clip. Returns download_url and expires_at."""
        body = {
            "camera_id": camera_id,
            "start_time": start_time.isoformat(),
            "end_time": end_time.isoformat(),
            "format": format,
        }
        return self._post("/v1/recordings/export", body)
