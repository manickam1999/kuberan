import asyncio
import os
import sys
from datetime import date
from types import SimpleNamespace

import httpx
import pytest
from telegram.error import Forbidden, NetworkError

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

os.environ.setdefault("TELEGRAM_BOT_TOKEN", "123456:TEST")
os.environ.setdefault("BOT_INTERNAL_SECRET", "test")

import main  # noqa: E402
import reminders  # noqa: E402
from api_client import KuberanAPIClient  # noqa: E402

TODAY = date(2026, 11, 9)

CARD = {
    "id": "r1", "kind": "card_due", "telegram_user_id": 777, "account_name": "Amex Gold",
    "due_on": "2026-12-01", "amount": 123450, "currency": "MYR",
}


# --- message formatting ---

def test_card_message():
    assert reminders.format_reminder_message(CARD, TODAY) == "💳 <b>Amex Gold</b> is due on 1 Dec — RM 1,234.50 owed"


def test_message_uses_the_accounts_own_currency():
    msg = reminders.format_reminder_message({**CARD, "currency": "USD", "amount": 50000}, TODAY)
    assert "US$500.00 owed" in msg
    assert "RM" not in msg


def test_loan_message_with_and_without_monthly_payment():
    loan = {**CARD, "kind": "loan_due", "account_name": "Car loan", "amount": 5000000, "due_on": "2026-11-15"}
    assert reminders.format_reminder_message(loan, TODAY) == "🏦 <b>Car loan</b> is due on 15 Nov — RM 50,000.00 owed"
    with_payment = reminders.format_reminder_message({**loan, "monthly_payment": 120000}, TODAY)
    assert with_payment == "🏦 <b>Car loan</b> payment of RM 1,200.00 is due on 15 Nov — RM 50,000.00 owed"


def test_fd_message():
    fd = {**CARD, "kind": "fd_maturity", "account_name": "FD 12M", "amount": 1040000, "due_on": "2027-06-01"}
    assert reminders.format_reminder_message(fd, TODAY) == "🏦 <b>FD 12M</b> matures on 1 Jun 2027 — RM 10,400.00 at maturity"


def test_account_names_are_html_escaped():
    msg = reminders.format_reminder_message({**CARD, "account_name": "A <b>&</b> B"}, TODAY)
    assert "A &lt;b&gt;&amp;&lt;/b&gt; B" in msg


@pytest.mark.parametrize("due_on,expected", [
    ("2026-12-01", "1 Dec"),
    ("2026-11-09", "9 Nov"),
    ("2027-01-31", "31 Jan 2027"),
])
def test_format_due_date(due_on, expected):
    assert reminders.format_due_date(due_on, TODAY) == expected


# --- the polling job ---

class FakeBot:
    def __init__(self, fail=None):
        self.sent = []
        self.fail = fail or {}

    async def send_message(self, chat_id, text, parse_mode=None):
        if chat_id in self.fail:
            raise self.fail[chat_id]
        self.sent.append((chat_id, text, parse_mode))


def _run_job(items, bot, confirm_status=200, due_status=200):
    """Run the job against a mock API; returns the reminder ids confirmed as sent."""
    confirmed = []

    def handler(request: httpx.Request) -> httpx.Response:
        assert request.headers.get("X-Internal-Secret") == "s3cret"
        if request.method == "GET" and request.url.path == "/api/v1/internal/telegram/reminders/due":
            if due_status != 200:
                return httpx.Response(due_status)
            return httpx.Response(200, json={"reminders": items})
        if request.method == "POST" and request.url.path.endswith("/sent"):
            confirmed.append(request.url.path.split("/")[-2])
            return httpx.Response(confirm_status, json={})
        return httpx.Response(404)

    async def run():
        api = KuberanAPIClient("http://api:8080", "s3cret", transport=httpx.MockTransport(handler))
        try:
            ctx = SimpleNamespace(job=SimpleNamespace(data=api), bot=bot)
            await reminders.send_due_reminders(ctx)
        finally:
            await api.aclose()

    asyncio.run(run())
    return confirmed


def test_job_sends_each_reminder_then_confirms_it():
    bot = FakeBot()
    items = [CARD, {**CARD, "id": "r2", "telegram_user_id": 888, "account_name": "Visa"}]
    confirmed = _run_job(items, bot)
    assert [s[0] for s in bot.sent] == [777, 888]
    assert bot.sent[0][2] == "HTML"
    assert "Amex Gold" in bot.sent[0][1]
    assert confirmed == ["r1", "r2"]


def test_job_with_nothing_due_sends_nothing():
    bot = FakeBot()
    assert _run_job([], bot) == []
    assert bot.sent == []


def test_transient_telegram_failure_is_not_confirmed_and_does_not_block_others():
    bot = FakeBot(fail={777: NetworkError("boom")})
    items = [CARD, {**CARD, "id": "r2", "telegram_user_id": 888}]
    confirmed = _run_job(items, bot)
    assert confirmed == ["r2"]
    assert [s[0] for s in bot.sent] == [888]


def test_blocked_user_is_confirmed_so_it_is_not_retried_forever():
    bot = FakeBot(fail={777: Forbidden("bot was blocked by the user")})
    assert _run_job([CARD], bot) == ["r1"]
    assert bot.sent == []


def test_api_error_while_fetching_is_swallowed():
    bot = FakeBot()
    assert _run_job([CARD], bot, due_status=500) == []
    assert bot.sent == []


def test_failed_confirmation_does_not_crash_the_job():
    bot = FakeBot()
    items = [CARD, {**CARD, "id": "r2", "telegram_user_id": 888}]
    confirmed = _run_job(items, bot, confirm_status=500)
    assert confirmed == ["r1", "r2"]  # both attempted
    assert len(bot.sent) == 2


# --- wiring ---

def test_reminder_job_is_registered():
    client = KuberanAPIClient("http://api:8080", "test")
    app = main.build_application(client)
    jobs = app.job_queue.get_jobs_by_name("reminders")
    assert len(jobs) == 1
    assert jobs[0].data is client
    asyncio.run(client.aclose())
