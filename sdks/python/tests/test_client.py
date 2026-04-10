"""Tests for the KaiVue Python SDK."""

from __future__ import annotations

import json

import pytest
import httpx
from pytest_httpx import HTTPXMock

from kaivue import KaiVueClient, Camera, CameraState, User
from kaivue.auth import APIKeyAuth, OAuthAuth
from kaivue.errors import NotFoundError, AuthenticationError, ValidationError


BASE_URL = "https://test.kaivue.io"


# ---------------------------------------------------------------------------
# Auth tests
# ---------------------------------------------------------------------------


class TestAPIKeyAuth:
    def test_apply_sets_header(self):
        auth = APIKeyAuth(api_key="test-key-123")
        req = httpx.Request("GET", "https://example.com")
        auth.apply(req)
        assert req.headers["X-API-Key"] == "test-key-123"


class TestOAuthAuth:
    def test_apply_sets_bearer(self):
        auth = OAuthAuth(access_token="tok-abc")
        req = httpx.Request("GET", "https://example.com")
        auth.apply(req)
        assert req.headers["Authorization"] == "Bearer tok-abc"


# ---------------------------------------------------------------------------
# Camera service tests
# ---------------------------------------------------------------------------


SAMPLE_CAMERA = {
    "id": "cam-001",
    "name": "Front Door",
    "description": "Main entrance",
    "manufacturer": "Hikvision",
    "model": "DS-2CD2143G2-I",
    "state": "CAMERA_STATE_ONLINE",
    "recording_mode": "RECORDING_MODE_CONTINUOUS",
    "ip_address": "192.168.1.10",
    "profiles": [{"name": "main", "codec": "h264", "width": 1920, "height": 1080}],
    "labels": ["entrance"],
    "recorder_id": "rec-01",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-01-15T10:00:00Z",
}


class TestCameraService:
    def test_list_cameras(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=httpx.URL(BASE_URL + "/v1/cameras", params={"page_size": "50"}),
            json={"cameras": [SAMPLE_CAMERA], "next_cursor": "", "total_count": 1},
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            cameras = client.cameras.list()
            assert len(cameras) == 1
            assert cameras[0].name == "Front Door"
            assert cameras[0].state == CameraState.ONLINE

    def test_get_camera(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=BASE_URL + "/v1/cameras/cam-001",
            json={"camera": SAMPLE_CAMERA},
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            cam = client.cameras.get("cam-001")
            assert cam.id == "cam-001"
            assert cam.manufacturer == "Hikvision"

    def test_create_camera(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=BASE_URL + "/v1/cameras",
            json={"camera": SAMPLE_CAMERA},
            status_code=200,
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            cam = client.cameras.create(
                name="Front Door",
                ip_address="192.168.1.10",
                recorder_id="rec-01",
            )
            assert cam.name == "Front Door"

    def test_delete_camera(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=httpx.URL(
                BASE_URL + "/v1/cameras/cam-001",
                params={"purge_recordings": "false"},
            ),
            status_code=204,
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            client.cameras.delete("cam-001")


# ---------------------------------------------------------------------------
# User service tests
# ---------------------------------------------------------------------------


SAMPLE_USER = {
    "id": "usr-001",
    "username": "jdoe",
    "email": "jdoe@example.com",
    "display_name": "Jane Doe",
    "groups": ["admin"],
    "disabled": False,
}


class TestUserService:
    def test_list_users(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=httpx.URL(BASE_URL + "/v1/users", params={"page_size": "50"}),
            json={"users": [SAMPLE_USER], "next_cursor": "", "total_count": 1},
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            users = client.users.list()
            assert len(users) == 1
            assert users[0].username == "jdoe"


# ---------------------------------------------------------------------------
# Error handling tests
# ---------------------------------------------------------------------------


class TestErrorHandling:
    def test_404_raises_not_found(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=BASE_URL + "/v1/cameras/missing",
            json={"message": "Camera not found", "request_id": "req-123"},
            status_code=404,
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            with pytest.raises(NotFoundError) as exc_info:
                client.cameras.get("missing")
            assert exc_info.value.status_code == 404
            assert exc_info.value.request_id == "req-123"

    def test_401_raises_auth_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=httpx.URL(BASE_URL + "/v1/cameras", params={"page_size": "50"}),
            json={"message": "Invalid API key"},
            status_code=401,
        )
        with KaiVueClient(BASE_URL, api_key="bad") as client:
            with pytest.raises(AuthenticationError):
                client.cameras.list()

    def test_422_raises_validation_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            url=BASE_URL + "/v1/cameras",
            json={
                "message": "Validation failed",
                "field_errors": [{"field": "name", "message": "required"}],
            },
            status_code=422,
        )
        with KaiVueClient(BASE_URL, api_key="k") as client:
            with pytest.raises(ValidationError) as exc_info:
                client.cameras.create(name="", ip_address="", recorder_id="")
            assert len(exc_info.value.field_errors) == 1


# ---------------------------------------------------------------------------
# Client init tests
# ---------------------------------------------------------------------------


class TestClientInit:
    def test_cannot_pass_both_api_key_and_auth(self):
        with pytest.raises(ValueError, match="not both"):
            KaiVueClient(BASE_URL, api_key="k", auth=OAuthAuth(access_token="t"))
