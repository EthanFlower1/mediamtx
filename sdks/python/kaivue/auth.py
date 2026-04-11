"""Authentication providers for the KaiVue SDK."""

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Optional

import httpx


class AuthProvider:
    """Base class for authentication providers."""

    def apply(self, request: httpx.Request) -> httpx.Request:
        """Apply authentication to an outgoing request."""
        raise NotImplementedError


@dataclass
class APIKeyAuth(AuthProvider):
    """Authenticate via a static API key sent in the X-API-Key header."""

    api_key: str

    def apply(self, request: httpx.Request) -> httpx.Request:
        request.headers["X-API-Key"] = self.api_key
        return request


@dataclass
class OAuthAuth(AuthProvider):
    """Authenticate via OAuth2 bearer token with automatic refresh.

    Supply either a static access_token or a client_credentials grant
    (client_id + client_secret + token_url) for automatic token refresh.
    """

    access_token: Optional[str] = None
    refresh_token: Optional[str] = None
    client_id: Optional[str] = None
    client_secret: Optional[str] = None
    token_url: Optional[str] = None
    _expires_at: float = field(default=0.0, init=False)
    _http: Optional[httpx.Client] = field(default=None, init=False)

    def apply(self, request: httpx.Request) -> httpx.Request:
        if self._needs_refresh():
            self._refresh()
        request.headers["Authorization"] = f"Bearer {self.access_token}"
        return request

    def _needs_refresh(self) -> bool:
        if not self.access_token:
            return True
        if self._expires_at and time.time() > self._expires_at - 30:
            return True
        return False

    def _refresh(self) -> None:
        if not self.token_url:
            raise ValueError("Cannot refresh: no token_url configured")

        if self._http is None:
            self._http = httpx.Client(timeout=10.0)

        if self.refresh_token:
            data = {
                "grant_type": "refresh_token",
                "refresh_token": self.refresh_token,
            }
        elif self.client_id and self.client_secret:
            data = {
                "grant_type": "client_credentials",
                "client_id": self.client_id,
                "client_secret": self.client_secret,
            }
        else:
            raise ValueError("Cannot refresh: need refresh_token or client_id+client_secret")

        resp = self._http.post(self.token_url, data=data)
        resp.raise_for_status()
        body = resp.json()
        self.access_token = body["access_token"]
        if "refresh_token" in body:
            self.refresh_token = body["refresh_token"]
        if "expires_in" in body:
            self._expires_at = time.time() + body["expires_in"]
