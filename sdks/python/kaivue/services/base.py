"""Base service with shared HTTP helpers."""

from __future__ import annotations

from typing import Any, Dict, Generator, List, Optional, Type, TypeVar

import httpx
from pydantic import BaseModel

from kaivue.errors import raise_for_status

T = TypeVar("T", bound=BaseModel)


class BaseService:
    """Shared HTTP helpers for all service clients."""

    def __init__(self, http: httpx.Client) -> None:
        self._http = http

    def _request(
        self,
        method: str,
        path: str,
        *,
        json: Optional[Dict[str, Any]] = None,
        params: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        # Strip None values from params
        if params:
            params = {k: v for k, v in params.items() if v is not None}

        resp = self._http.request(method, path, json=json, params=params)
        request_id = resp.headers.get("X-Request-Id")

        if resp.status_code >= 400:
            try:
                body = resp.json()
            except Exception:
                body = {"message": resp.text}
            raise_for_status(resp.status_code, body, request_id)

        if resp.status_code == 204 or not resp.content:
            return {}
        return resp.json()

    def _get(self, path: str, **params: Any) -> Dict[str, Any]:
        return self._request("GET", path, params=params)

    def _post(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("POST", path, json=body)

    def _put(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("PUT", path, json=body)

    def _patch(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("PATCH", path, json=body)

    def _delete(self, path: str) -> Dict[str, Any]:
        return self._request("DELETE", path)

    def _parse(self, model: Type[T], data: Dict[str, Any]) -> T:
        return model.model_validate(data)

    def _parse_list(self, model: Type[T], items: List[Dict[str, Any]]) -> List[T]:
        return [model.model_validate(item) for item in items]

    def _paginate(
        self,
        model: Type[T],
        path: str,
        list_key: str,
        *,
        page_size: int = 50,
        **params: Any,
    ) -> Generator[T, None, None]:
        """Auto-paginate through a list endpoint, yielding each item."""
        cursor: Optional[str] = None
        while True:
            resp = self._get(path, page_size=page_size, cursor=cursor, **params)
            items = resp.get(list_key, [])
            for item in items:
                yield model.model_validate(item)
            cursor = resp.get("next_cursor")
            if not cursor or not items:
                break
