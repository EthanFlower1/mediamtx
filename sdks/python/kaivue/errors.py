"""Error types for the KaiVue SDK.

Error codes align with the API's gRPC/Connect error model:
  - 401 -> AuthenticationError
  - 403 -> AuthenticationError (permission denied)
  - 404 -> NotFoundError
  - 400/422 -> ValidationError
  - 429 -> RateLimitError
  - 500+ -> ServerError
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Optional


@dataclass
class FieldError:
    """A single field-level validation error."""

    field: str
    message: str


@dataclass
class KaiVueError(Exception):
    """Base exception for all KaiVue SDK errors."""

    message: str
    status_code: Optional[int] = None
    request_id: Optional[str] = None
    field_errors: List[FieldError] = field(default_factory=list)

    def __str__(self) -> str:
        parts = [self.message]
        if self.status_code:
            parts.insert(0, f"[{self.status_code}]")
        if self.request_id:
            parts.append(f"(request_id={self.request_id})")
        return " ".join(parts)


class AuthenticationError(KaiVueError):
    """Raised on 401/403 responses."""

    pass


class NotFoundError(KaiVueError):
    """Raised on 404 responses."""

    pass


class ValidationError(KaiVueError):
    """Raised on 400/422 responses with field-level errors."""

    pass


class RateLimitError(KaiVueError):
    """Raised on 429 responses. Check retry_after for backoff hint."""

    retry_after: Optional[float] = None


class ServerError(KaiVueError):
    """Raised on 500+ responses."""

    pass


def raise_for_status(status_code: int, body: dict, request_id: str | None = None) -> None:
    """Map an HTTP error response to the appropriate SDK exception."""
    msg = body.get("message", body.get("error", "Unknown error"))
    field_errors = [
        FieldError(field=fe["field"], message=fe["message"])
        for fe in body.get("field_errors", [])
    ]
    kwargs = dict(
        message=msg,
        status_code=status_code,
        request_id=request_id or body.get("request_id"),
        field_errors=field_errors,
    )

    if status_code in (401, 403):
        raise AuthenticationError(**kwargs)
    elif status_code == 404:
        raise NotFoundError(**kwargs)
    elif status_code in (400, 422):
        raise ValidationError(**kwargs)
    elif status_code == 429:
        raise RateLimitError(**kwargs)
    elif status_code >= 500:
        raise ServerError(**kwargs)
    else:
        raise KaiVueError(**kwargs)
