"""Retention policy service client."""

from __future__ import annotations

from typing import Any, Dict, List, Optional

from kaivue.models.retention import RetentionPolicy
from kaivue.services.base import BaseService


class RetentionService(BaseService):
    """CRUD operations for retention policies."""

    def create(
        self,
        name: str,
        retention_days: int,
        *,
        description: str = "",
        max_bytes: int = 0,
        event_retention_days: int = 0,
    ) -> RetentionPolicy:
        body: Dict[str, Any] = {
            "name": name,
            "retention_days": retention_days,
        }
        if description:
            body["description"] = description
        if max_bytes:
            body["max_bytes"] = max_bytes
        if event_retention_days:
            body["event_retention_days"] = event_retention_days
        resp = self._post("/v1/retention-policies", body)
        return self._parse(RetentionPolicy, resp["policy"])

    def get(self, policy_id: str) -> RetentionPolicy:
        resp = self._get(f"/v1/retention-policies/{policy_id}")
        return self._parse(RetentionPolicy, resp["policy"])

    def update(
        self,
        policy_id: str,
        *,
        name: Optional[str] = None,
        description: Optional[str] = None,
        retention_days: Optional[int] = None,
        max_bytes: Optional[int] = None,
        event_retention_days: Optional[int] = None,
    ) -> RetentionPolicy:
        body: Dict[str, Any] = {"id": policy_id}
        mask: List[str] = []
        if name is not None:
            body["name"] = name
            mask.append("name")
        if description is not None:
            body["description"] = description
            mask.append("description")
        if retention_days is not None:
            body["retention_days"] = retention_days
            mask.append("retention_days")
        if max_bytes is not None:
            body["max_bytes"] = max_bytes
            mask.append("max_bytes")
        if event_retention_days is not None:
            body["event_retention_days"] = event_retention_days
            mask.append("event_retention_days")
        body["update_mask"] = mask

        resp = self._patch(f"/v1/retention-policies/{policy_id}", body)
        return self._parse(RetentionPolicy, resp["policy"])

    def delete(self, policy_id: str) -> None:
        self._delete(f"/v1/retention-policies/{policy_id}")

    def list(
        self, *, page_size: int = 50, cursor: Optional[str] = None
    ) -> List[RetentionPolicy]:
        params: Dict[str, Any] = {"page_size": page_size}
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/retention-policies", **params)
        return self._parse_list(RetentionPolicy, resp.get("policies", []))

    def apply(self, policy_id: str, camera_ids: List[str]) -> RetentionPolicy:
        """Apply a retention policy to one or more cameras."""
        body = {"policy_id": policy_id, "camera_ids": camera_ids}
        resp = self._post(f"/v1/retention-policies/{policy_id}/apply", body)
        return self._parse(RetentionPolicy, resp["policy"])
