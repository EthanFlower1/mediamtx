"""KaiVue SDK main client."""

from __future__ import annotations

from typing import Optional

import httpx

from kaivue.auth import APIKeyAuth, AuthProvider, OAuthAuth
from kaivue.services.cameras import CameraService
from kaivue.services.users import UserService
from kaivue.services.recordings import RecordingService
from kaivue.services.events import EventService
from kaivue.services.schedules import ScheduleService
from kaivue.services.retention import RetentionService
from kaivue.services.integrations import IntegrationService

_SDK_VERSION = "0.1.0"


class KaiVueClient:
    """KaiVue VMS API client.

    Provides access to all 7 API services: cameras, users, recordings,
    events, schedules, retention, and integrations.

    Authentication:
        - API Key: ``KaiVueClient(base_url, api_key="your-key")``
        - OAuth:   ``KaiVueClient(base_url, auth=OAuthAuth(access_token="..."))``

    Example::

        client = KaiVueClient("https://your-instance.kaivue.io", api_key="key")
        cameras = client.cameras.list()
        for cam in cameras:
            print(cam.name, cam.state)
    """

    def __init__(
        self,
        base_url: str,
        *,
        api_key: Optional[str] = None,
        auth: Optional[AuthProvider] = None,
        timeout: float = 30.0,
    ) -> None:
        if api_key and auth:
            raise ValueError("Provide either api_key or auth, not both")
        if api_key:
            auth = APIKeyAuth(api_key=api_key)

        self._auth = auth
        self._http = httpx.Client(
            base_url=base_url.rstrip("/"),
            timeout=timeout,
            headers={
                "User-Agent": f"kaivue-python/{_SDK_VERSION}",
                "Accept": "application/json",
                "Content-Type": "application/json",
            },
            event_hooks={"request": [self._inject_auth]},
        )

        # Service accessors
        self.cameras = CameraService(self._http)
        self.users = UserService(self._http)
        self.recordings = RecordingService(self._http)
        self.events = EventService(self._http)
        self.schedules = ScheduleService(self._http)
        self.retention = RetentionService(self._http)
        self.integrations = IntegrationService(self._http)

    def _inject_auth(self, request: httpx.Request) -> None:
        if self._auth:
            self._auth.apply(request)

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._http.close()

    def __enter__(self) -> KaiVueClient:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()


class AsyncKaiVueClient:
    """Async variant of KaiVueClient using httpx.AsyncClient.

    Usage::

        async with AsyncKaiVueClient("https://...", api_key="key") as client:
            cameras = await client.get("/v1/cameras")
    """

    def __init__(
        self,
        base_url: str,
        *,
        api_key: Optional[str] = None,
        auth: Optional[AuthProvider] = None,
        timeout: float = 30.0,
    ) -> None:
        if api_key and auth:
            raise ValueError("Provide either api_key or auth, not both")
        if api_key:
            auth = APIKeyAuth(api_key=api_key)

        self._auth = auth
        self._http = httpx.AsyncClient(
            base_url=base_url.rstrip("/"),
            timeout=timeout,
            headers={
                "User-Agent": f"kaivue-python/{_SDK_VERSION}",
                "Accept": "application/json",
                "Content-Type": "application/json",
            },
            event_hooks={"request": [self._inject_auth]},
        )

    async def _inject_auth(self, request: httpx.Request) -> None:
        if self._auth:
            self._auth.apply(request)

    async def get(self, path: str, **params: object) -> dict:
        resp = await self._http.get(path, params=params)
        resp.raise_for_status()
        return resp.json()

    async def post(self, path: str, body: dict) -> dict:
        resp = await self._http.post(path, json=body)
        resp.raise_for_status()
        return resp.json()

    async def close(self) -> None:
        await self._http.aclose()

    async def __aenter__(self) -> AsyncKaiVueClient:
        return self

    async def __aexit__(self, *args: object) -> None:
        await self.close()
