"""Schedule service client."""

from __future__ import annotations

from typing import Any, Dict, List, Optional

from kaivue.models.schedule import Schedule, ScheduleEntry
from kaivue.services.base import BaseService


class ScheduleService(BaseService):
    """CRUD operations for recording schedules."""

    def create(
        self,
        camera_id: str,
        name: str,
        timezone: str,
        entries: List[ScheduleEntry],
    ) -> Schedule:
        body = {
            "camera_id": camera_id,
            "name": name,
            "timezone": timezone,
            "entries": [e.model_dump() for e in entries],
        }
        resp = self._post("/v1/schedules", body)
        return self._parse(Schedule, resp["schedule"])

    def get(self, schedule_id: str) -> Schedule:
        resp = self._get(f"/v1/schedules/{schedule_id}")
        return self._parse(Schedule, resp["schedule"])

    def update(
        self,
        schedule_id: str,
        *,
        name: Optional[str] = None,
        timezone: Optional[str] = None,
        entries: Optional[List[ScheduleEntry]] = None,
    ) -> Schedule:
        body: Dict[str, Any] = {"id": schedule_id}
        mask: List[str] = []
        if name is not None:
            body["name"] = name
            mask.append("name")
        if timezone is not None:
            body["timezone"] = timezone
            mask.append("timezone")
        if entries is not None:
            body["entries"] = [e.model_dump() for e in entries]
            mask.append("entries")
        body["update_mask"] = mask

        resp = self._patch(f"/v1/schedules/{schedule_id}", body)
        return self._parse(Schedule, resp["schedule"])

    def delete(self, schedule_id: str) -> None:
        self._delete(f"/v1/schedules/{schedule_id}")

    def list(
        self,
        *,
        camera_id: Optional[str] = None,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[Schedule]:
        params: Dict[str, Any] = {"page_size": page_size}
        if camera_id:
            params["camera_id"] = camera_id
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/schedules", **params)
        return self._parse_list(Schedule, resp.get("schedules", []))
