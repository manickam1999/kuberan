import asyncio
import os
import sys

import httpx

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api_client import UserAPIClient  # noqa: E402
from handlers.common import get_base_currency  # noqa: E402


def _user_client(handler):
    http = httpx.AsyncClient(base_url="http://api:8080", transport=httpx.MockTransport(handler))
    return UserAPIClient(http, "jwt")


def test_base_currency_comes_from_profile():
    def handler(request):
        assert request.url.path == "/api/v1/profile"
        return httpx.Response(200, json={"user": {"base_currency": "USD"}})

    async def run():
        return await get_base_currency(_user_client(handler))

    assert asyncio.run(run()) == "USD"


def test_base_currency_falls_back_to_myr_when_profile_fails():
    async def run():
        return await get_base_currency(_user_client(lambda r: httpx.Response(500)))

    assert asyncio.run(run()) == "MYR"
