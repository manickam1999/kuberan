import asyncio
import os
import sys

import httpx
import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api_client import KuberanAPIClient  # noqa: E402


def _client(handler):
    return KuberanAPIClient("http://api:8080/", "s3cret", transport=httpx.MockTransport(handler))


def test_resolve_user_sends_internal_secret_and_returns_json():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["url"] = str(request.url)
        seen["secret"] = request.headers.get("X-Internal-Secret")
        return httpx.Response(200, json={"user_id": "u1", "auth_token": "tok"})

    async def run():
        client = _client(handler)
        try:
            return await client.resolve_user(42)
        finally:
            await client.aclose()

    assert asyncio.run(run()) == {"user_id": "u1", "auth_token": "tok"}
    assert seen["url"] == "http://api:8080/api/v1/internal/telegram/resolve/42"
    assert seen["secret"] == "s3cret"


def test_resolve_user_not_linked_returns_none():
    async def run():
        client = _client(lambda r: httpx.Response(404, json={"error": "not found"}))
        try:
            return await client.resolve_user(42)
        finally:
            await client.aclose()

    assert asyncio.run(run()) is None


def test_resolve_user_server_error_raises():
    async def run():
        client = _client(lambda r: httpx.Response(500))
        try:
            await client.resolve_user(42)
        finally:
            await client.aclose()

    with pytest.raises(httpx.HTTPStatusError):
        asyncio.run(run())


def test_record_activity_swallows_errors():
    def handler(request):
        raise httpx.ConnectError("down", request=request)

    async def run():
        client = _client(handler)
        try:
            await client.record_activity(42)
        finally:
            await client.aclose()

    asyncio.run(run())  # must not raise


def test_user_client_uses_bearer_token_and_shared_pool():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["auth"] = request.headers.get("Authorization")
        seen["secret"] = request.headers.get("X-Internal-Secret")
        seen["params"] = dict(request.url.params)
        return httpx.Response(200, json={"data": [{"id": "a1"}]})

    async def run():
        client = _client(handler)
        try:
            user = client.for_user("jwt-123")
            assert user.http is client.http
            return await user.get_accounts()
        finally:
            await client.aclose()

    assert asyncio.run(run()) == [{"id": "a1"}]
    assert seen["auth"] == "Bearer jwt-123"
    assert seen["secret"] is None  # internal secret never sent on user calls
    assert seen["params"] == {"page_size": "100"}
