"""Camera service client."""

from __future__ import annotations

from typing import Any, Dict, Generator, List, Optional

from kaivue.models.camera import Camera, CameraState, RecordingMode
from kaivue.services.base import BaseService


class CameraService(BaseService):
    """CRUD operations for cameras."""

    def create(
        self,
        name: str,
        ip_address: str,
        recorder_id: str,
        *,
        description: str = "",
        recording_mode: Optional[RecordingMode] = None,
        labels: Optional[List[str]] = None,
        username: str = "",
        password: str = "",
    ) -> Camera:
        body: Dict[str, Any] = {
            "name": name,
            "ip_address": ip_address,
            "recorder_id": recorder_id,
        }
        if description:
            body["description"] = description
        if recording_mode:
            body["recording_mode"] = recording_mode.value
        if labels:
            body["labels"] = labels
        if username:
            body["username"] = username
        if password:
            body["password"] = password

        resp = self._post("/v1/cameras", body)
        return self._parse(Camera, resp["camera"])

    def get(self, camera_id: str) -> Camera:
        resp = self._get(f"/v1/cameras/{camera_id}")
        return self._parse(Camera, resp["camera"])

    def update(
        self,
        camera_id: str,
        *,
        name: Optional[str] = None,
        description: Optional[str] = None,
        recording_mode: Optional[RecordingMode] = None,
        labels: Optional[List[str]] = None,
        audio_enabled: Optional[bool] = None,
        motion_sensitivity: Optional[int] = None,
    ) -> Camera:
        body: Dict[str, Any] = {"id": camera_id}
        mask: List[str] = []
        if name is not None:
            body["name"] = name
            mask.append("name")
        if description is not None:
            body["description"] = description
            mask.append("description")
        if recording_mode is not None:
            body["recording_mode"] = recording_mode.value
            mask.append("recording_mode")
        if labels is not None:
            body["labels"] = labels
            mask.append("labels")
        if audio_enabled is not None:
            body["audio_enabled"] = audio_enabled
            mask.append("audio_enabled")
        if motion_sensitivity is not None:
            body["motion_sensitivity"] = motion_sensitivity
            mask.append("motion_sensitivity")
        body["update_mask"] = mask

        resp = self._patch(f"/v1/cameras/{camera_id}", body)
        return self._parse(Camera, resp["camera"])

    def delete(self, camera_id: str, *, purge_recordings: bool = False) -> None:
        self._request(
            "DELETE",
            f"/v1/cameras/{camera_id}",
            params={"purge_recordings": purge_recordings},
        )

    def list(
        self,
        *,
        search: Optional[str] = None,
        recorder_id: Optional[str] = None,
        state_filter: Optional[CameraState] = None,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[Camera]:
        params: Dict[str, Any] = {"page_size": page_size}
        if search:
            params["search"] = search
        if recorder_id:
            params["recorder_id"] = recorder_id
        if state_filter:
            params["state_filter"] = state_filter.value
        if cursor:
            params["cursor"] = cursor

        resp = self._get("/v1/cameras", **params)
        return self._parse_list(Camera, resp.get("cameras", []))

    def list_all(
        self,
        *,
        search: Optional[str] = None,
        recorder_id: Optional[str] = None,
        state_filter: Optional[CameraState] = None,
        page_size: int = 50,
    ) -> Generator[Camera, None, None]:
        """Auto-paginate through all cameras."""
        params: Dict[str, Any] = {}
        if search:
            params["search"] = search
        if recorder_id:
            params["recorder_id"] = recorder_id
        if state_filter:
            params["state_filter"] = state_filter.value
        yield from self._paginate(Camera, "/v1/cameras", "cameras", page_size=page_size, **params)
