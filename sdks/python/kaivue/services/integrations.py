"""Integration service client."""

from __future__ import annotations

from typing import Any, Dict, List, Optional

from kaivue.models.event import EventKind
from kaivue.models.integration import Integration, IntegrationKind
from kaivue.services.base import BaseService


class IntegrationService(BaseService):
    """CRUD operations for integrations (webhooks, MQTT, etc.)."""

    def create(
        self,
        name: str,
        kind: IntegrationKind,
        *,
        config: Optional[Dict[str, str]] = None,
        subscribed_events: Optional[List[EventKind]] = None,
        camera_ids: Optional[List[str]] = None,
    ) -> Integration:
        body: Dict[str, Any] = {
            "name": name,
            "kind": kind.value,
        }
        if config:
            body["config"] = config
        if subscribed_events:
            body["subscribed_events"] = [e.value for e in subscribed_events]
        if camera_ids:
            body["camera_ids"] = camera_ids
        resp = self._post("/v1/integrations", body)
        return self._parse(Integration, resp["integration"])

    def get(self, integration_id: str) -> Integration:
        resp = self._get(f"/v1/integrations/{integration_id}")
        return self._parse(Integration, resp["integration"])

    def update(
        self,
        integration_id: str,
        *,
        name: Optional[str] = None,
        enabled: Optional[bool] = None,
        config: Optional[Dict[str, str]] = None,
        subscribed_events: Optional[List[EventKind]] = None,
        camera_ids: Optional[List[str]] = None,
    ) -> Integration:
        body: Dict[str, Any] = {"id": integration_id}
        mask: List[str] = []
        if name is not None:
            body["name"] = name
            mask.append("name")
        if enabled is not None:
            body["enabled"] = enabled
            mask.append("enabled")
        if config is not None:
            body["config"] = config
            mask.append("config")
        if subscribed_events is not None:
            body["subscribed_events"] = [e.value for e in subscribed_events]
            mask.append("subscribed_events")
        if camera_ids is not None:
            body["camera_ids"] = camera_ids
            mask.append("camera_ids")
        body["update_mask"] = mask

        resp = self._patch(f"/v1/integrations/{integration_id}", body)
        return self._parse(Integration, resp["integration"])

    def delete(self, integration_id: str) -> None:
        self._delete(f"/v1/integrations/{integration_id}")

    def list(
        self,
        *,
        kind_filter: Optional[IntegrationKind] = None,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[Integration]:
        params: Dict[str, Any] = {"page_size": page_size}
        if kind_filter:
            params["kind_filter"] = kind_filter.value
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/integrations", **params)
        return self._parse_list(Integration, resp.get("integrations", []))

    def test(self, integration_id: str) -> Dict[str, Any]:
        """Test an integration connectivity. Returns success, message, latency_ms."""
        return self._post(f"/v1/integrations/{integration_id}/test", {})
