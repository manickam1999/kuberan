import asyncio
import os
import sys
from types import SimpleNamespace

import pytest
from telegram.ext import ApplicationHandlerStop

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

os.environ.setdefault("TELEGRAM_BOT_TOKEN", "123456:TEST")
os.environ.setdefault("BOT_INTERNAL_SECRET", "test")

import main  # noqa: E402
from api_client import KuberanAPIClient  # noqa: E402


class FakeUpdate(main.Update):
    """Update stand-in whose effective_chat can be set directly."""

    def __init__(self, chat_type):
        super().__init__(update_id=1)
        self._chat = SimpleNamespace(type=chat_type) if chat_type else None

    @property
    def effective_chat(self):
        return self._chat


@pytest.mark.parametrize("chat_type", ["group", "supergroup", "channel", None])
def test_non_private_updates_are_dropped(chat_type):
    with pytest.raises(ApplicationHandlerStop):
        asyncio.run(main.reject_non_private(FakeUpdate(chat_type), None))


def test_private_updates_pass():
    asyncio.run(main.reject_non_private(FakeUpdate("private"), None))


def test_httpx_logging_is_silenced():
    import logging
    assert logging.getLogger("httpx").getEffectiveLevel() >= logging.WARNING


def test_application_has_job_queue_and_error_handler():
    client = KuberanAPIClient("http://api:8080", "test")
    app = main.build_application(client)
    assert app.job_queue is not None
    assert app.error_handlers
    asyncio.run(client.aclose())
