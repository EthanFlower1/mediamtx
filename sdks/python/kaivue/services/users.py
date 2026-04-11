"""User service client."""

from __future__ import annotations

from typing import Any, Dict, Generator, List, Optional

from kaivue.models.user import User
from kaivue.services.base import BaseService


class UserService(BaseService):
    """CRUD operations for users."""

    def create(
        self,
        username: str,
        email: str,
        password: str,
        *,
        display_name: str = "",
        groups: Optional[List[str]] = None,
    ) -> User:
        body: Dict[str, Any] = {
            "username": username,
            "email": email,
            "password": password,
        }
        if display_name:
            body["display_name"] = display_name
        if groups:
            body["groups"] = groups

        resp = self._post("/v1/users", body)
        return self._parse(User, resp["user"])

    def get(self, user_id: str) -> User:
        resp = self._get(f"/v1/users/{user_id}")
        return self._parse(User, resp["user"])

    def update(
        self,
        user_id: str,
        *,
        email: Optional[str] = None,
        display_name: Optional[str] = None,
        groups: Optional[List[str]] = None,
        disabled: Optional[bool] = None,
    ) -> User:
        body: Dict[str, Any] = {"id": user_id}
        mask: List[str] = []
        if email is not None:
            body["email"] = email
            mask.append("email")
        if display_name is not None:
            body["display_name"] = display_name
            mask.append("display_name")
        if groups is not None:
            body["groups"] = groups
            mask.append("groups")
        if disabled is not None:
            body["disabled"] = disabled
            mask.append("disabled")
        body["update_mask"] = mask

        resp = self._patch(f"/v1/users/{user_id}", body)
        return self._parse(User, resp["user"])

    def delete(self, user_id: str) -> None:
        self._delete(f"/v1/users/{user_id}")

    def list(
        self,
        *,
        search: Optional[str] = None,
        page_size: int = 50,
        cursor: Optional[str] = None,
    ) -> List[User]:
        params: Dict[str, Any] = {"page_size": page_size}
        if search:
            params["search"] = search
        if cursor:
            params["cursor"] = cursor
        resp = self._get("/v1/users", **params)
        return self._parse_list(User, resp.get("users", []))

    def list_all(
        self, *, search: Optional[str] = None, page_size: int = 50
    ) -> Generator[User, None, None]:
        params: Dict[str, Any] = {}
        if search:
            params["search"] = search
        yield from self._paginate(User, "/v1/users", "users", page_size=page_size, **params)
