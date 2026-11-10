import asyncio
import os
import sys
from datetime import datetime, timedelta, timezone
from types import SimpleNamespace

import httpx
import pytest
from telegram.ext import ConversationHandler

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers import quick_add, receipts, spent, undo  # noqa: E402
from handlers.recent import describe_transaction, parse_timestamp, recent_bot_transactions  # noqa: E402
from handlers.transaction_flow import ACCOUNT, TXN_KEY, create_transaction_conversation  # noqa: E402

ACCOUNTS = [
    {"id": "acc-dbs", "name": "DBS Everyday", "type": "cash", "currency": "MYR", "is_active": True},
    {"id": "acc-usd", "name": "Wise", "type": "cash", "currency": "USD", "is_active": True},
    {"id": "acc-inv", "name": "Broker", "type": "investment", "currency": "USD", "is_active": True},
]
CATEGORIES = [
    {"id": "cat-din", "name": "Dining", "type": "expense", "icon": ""},
    {"id": "cat-tra", "name": "Transport", "type": "expense", "icon": ""},
    {"id": "cat-sal", "name": "Salary", "type": "income", "icon": ""},
]


def run(coro):
    return asyncio.run(coro)


class FakeUserClient:
    def __init__(self, accounts=ACCOUNTS, categories=CATEGORIES, default_account_id="acc-dbs",
                 suggestion=None, transactions=None):
        self.accounts = accounts
        self.categories = categories
        self.default_account_id = default_account_id
        self.suggestion = suggestion
        self.transactions = transactions or []
        self.created = []
        self.updated = []
        self.deleted = []
        self.uploads = []
        self.suggest_calls = []
        self.fail_create = False

    async def get_accounts(self):
        return self.accounts

    async def get_categories(self):
        return self.categories

    async def get_profile(self):
        return {"default_account_id": self.default_account_id, "base_currency": "MYR"}

    async def suggest_category(self, description, txn_type, amount=None, account_id=None):
        self.suggest_calls.append((description, txn_type, amount, account_id))
        return self.suggestion

    async def create_transaction(self, data):
        if self.fail_create:
            raise httpx.ConnectError("down")
        self.created.append(data)
        category_id = data.get("category_id")
        if not category_id and self.suggestion and self.suggestion.get("reason") == "rule":
            category_id = self.suggestion["category_id"]  # the API applies rules itself
        return {"transaction": {"id": f"tx-{len(self.created)}", "account_id": data["account_id"],
                                "category_id": category_id, "amount": data["amount"]}}

    async def get_transaction(self, transaction_id):
        return self.created_by_id(transaction_id)

    def created_by_id(self, transaction_id):
        merged = {"id": transaction_id, "amount": 1840, "account_id": "acc-dbs", "category_id": None}
        for tid, data in self.updated:
            if tid == transaction_id:
                merged.update(data)
        return merged

    async def update_transaction(self, transaction_id, data):
        self.updated.append((transaction_id, data))
        return self.created_by_id(transaction_id)

    async def delete_transaction(self, transaction_id):
        self.deleted.append(transaction_id)

    async def get_recent_transactions(self, page_size=50):
        return self.transactions

    async def upload_attachment(self, transaction_id, content, filename, content_type):
        self.uploads.append((transaction_id, content, filename, content_type))
        return {"id": "att-1"}

    async def get_spending_by_category(self, from_date, to_date):
        self.spending_range = (from_date, to_date)
        return self.spending


class FakeAPI:
    def __init__(self, user_client):
        self.user_client = user_client

    async def resolve_user(self, telegram_user_id):
        return {"user_id": "u1", "auth_token": "tok", "default_currency": "MYR"}

    async def record_activity(self, telegram_user_id):
        return None

    def for_user(self, token):
        return self.user_client


class FakeMessage:
    def __init__(self, text=None, user_id=7, **extra):
        self.text = text
        self.from_user = SimpleNamespace(id=user_id)
        self.replies = []
        for key, value in extra.items():
            setattr(self, key, value)

    async def reply_text(self, text, reply_markup=None, parse_mode=None):
        self.replies.append(SimpleNamespace(text=text, markup=reply_markup, parse_mode=parse_mode))
        return SimpleNamespace(message_id=100 + len(self.replies))


def make_update(text, update_id=1, **extra):
    message = FakeMessage(text, **extra)
    return SimpleNamespace(update_id=update_id, message=message, effective_user=message.from_user,
                           effective_message=message), message


def make_context(user_data=None):
    return SimpleNamespace(
        user_data=user_data if user_data is not None else {},
        application=SimpleNamespace(bot_data={}),
        args=[],
        bot=None,
    )


def button_data(markup):
    return [b.callback_data for row in markup.inline_keyboard for b in row]


def quick_add_handler(user_client):
    return quick_add.quick_add_entry_point(FakeAPI(user_client)).callback


def pending_id_of(message):
    """The pending id embedded in the preview's Add button."""
    return button_data(message.replies[-1].markup)[0].split(":")[2]


def preview(client, text, context=None, update_id=1):
    """Send the text; returns (context, preview message, pending id)."""
    update, message = make_update(text, update_id=update_id)
    context = context or make_context()
    run(quick_add_handler(client)(update, context))
    return context, message, pending_id_of(message)


def tap(client, context, data):
    handler = quick_add.quick_add_callbacks(FakeAPI(client)).callback
    query = FakeQuery(data)
    run(handler(SimpleNamespace(callback_query=query), context))
    return query


# --- 6.1 quick add ------------------------------------------------------------

def test_quick_add_shows_a_preview_and_creates_nothing():
    client = FakeUserClient(suggestion={"category_id": "cat-din", "category_name": "Dining", "reason": "history"})
    update, message = make_update("coffee 18.40")
    context = make_context()

    state = run(quick_add_handler(client)(update, context))

    assert state == ConversationHandler.END
    assert client.created == []
    reply = message.replies[-1]
    assert reply.text == "<b>Add RM 18.40</b> · Dining · DBS Everyday?"
    pid = pending_id_of(message)
    assert button_data(reply.markup) == [f"qa:pa:{pid}", f"qa:pc:{pid}", f"qa:pk:{pid}", f"qa:px:{pid}"]
    assert [b.text for row in reply.markup.inline_keyboard for b in row] == [
        "Add", "Change category", "Change account", "Cancel"]
    assert TXN_KEY not in context.user_data  # nothing left behind in the conversation


def test_add_creates_the_expense_once_and_offers_undo():
    client = FakeUserClient(suggestion={"category_id": "cat-din", "category_name": "Dining", "reason": "history"})
    context, message, pid = preview(client, "coffee 18.40")

    query = tap(client, context, f"qa:pa:{pid}")

    assert client.created == [{
        "type": "expense", "account_id": "acc-dbs", "amount": 1840,
        "description": "coffee", "category_id": "cat-din",
    }]
    assert query.edits[-1].text == "<b>Added RM 18.40</b> · Dining · DBS Everyday"
    assert button_data(query.edits[-1].markup) == ["qa:u:tx-1"]


def test_double_tap_on_add_never_creates_twice():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")

    tap(client, context, f"qa:pa:{pid}")
    second = tap(client, context, f"qa:pa:{pid}")

    assert len(client.created) == 1
    assert second.edits == []  # the "Added" message is not overwritten
    assert second.alerts == [quick_add.EXPIRED_MESSAGE]


def test_concurrent_add_taps_create_once():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")
    handler = quick_add.quick_add_callbacks(FakeAPI(client)).callback

    async def both():
        await asyncio.gather(*(
            handler(SimpleNamespace(callback_query=FakeQuery(f"qa:pa:{pid}")), context) for _ in range(2)
        ))

    run(both())
    assert len(client.created) == 1


def test_rule_suggestion_is_shown_but_not_sent():
    client = FakeUserClient(suggestion={"category_id": "cat-tra", "category_name": "Transport", "reason": "rule"})
    context, message, pid = preview(client, "18.40 grab")
    assert message.replies[-1].text == "<b>Add RM 18.40</b> · Transport · DBS Everyday?"

    query = tap(client, context, f"qa:pa:{pid}")

    assert "category_id" not in client.created[0]  # POST /transactions applies the rule itself
    assert query.edits[-1].text == "<b>Added RM 18.40</b> · Transport · DBS Everyday"


def test_no_suggestion_means_uncategorized():
    client = FakeUserClient()
    context, message, pid = preview(client, "coffee 18.40")
    assert message.replies[-1].text == "<b>Add RM 18.40</b> · Uncategorized · DBS Everyday?"

    tap(client, context, f"qa:pa:{pid}")

    assert "category_id" not in client.created[0]


def test_trailing_account_and_category_names_are_matched():
    client = FakeUserClient(accounts=[
        {"id": "acc-dbs", "name": "dbs", "type": "cash", "currency": "MYR", "is_active": True},
        {"id": "acc-maybank", "name": "maybank", "type": "cash", "currency": "MYR", "is_active": True},
    ], default_account_id="acc-dbs")
    context, _, pid = preview(client, "18.40 coffee dining maybank")  # account last, then category

    assert client.suggest_calls == []  # an explicit category needs no suggestion
    tap(client, context, f"qa:pa:{pid}")

    assert client.created[0]["account_id"] == "acc-maybank"
    assert client.created[0]["category_id"] == "cat-din"
    assert client.created[0]["description"] == "coffee"


def test_typed_currency_different_from_the_account_is_called_out():
    client = FakeUserClient()
    context, message, pid = preview(client, "lunch USD 12")

    text = message.replies[-1].text
    assert text.startswith("<b>Add RM 12.00</b>")
    assert "You typed USD" in text and "MYR" in text and "not converted" in text
    query = tap(client, context, f"qa:pa:{pid}")
    assert client.created[0]["amount"] == 1200
    assert "not converted" in query.edits[-1].text


def test_matching_currency_adds_no_note():
    client = FakeUserClient()
    _, message, _ = preview(client, "RM18.40 coffee")

    assert "not converted" not in message.replies[-1].text


def test_see_you_at_5_only_produces_a_preview():
    client = FakeUserClient()
    update, message = make_update("see you at 5")

    run(quick_add_handler(client)(update, make_context()))

    assert client.created == []
    assert message.replies[-1].text.startswith("<b>Add RM 5.00</b>")
    assert "?" in message.replies[-1].text


def test_cancel_creates_nothing_and_removes_the_buttons():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")

    query = tap(client, context, f"qa:px:{pid}")

    assert query.edits[-1].text == "Not added."
    assert query.edits[-1].markup is None
    assert client.created == []
    late = tap(client, context, f"qa:pa:{pid}")  # a tap on the old Add button
    assert client.created == []
    assert late.alerts == [quick_add.EXPIRED_MESSAGE]


def test_change_category_then_add_sends_the_picked_category():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")

    picker = tap(client, context, f"qa:pc:{pid}")
    assert button_data(picker.markups[-1]) == [f"qa:psc:{pid}:0", f"qa:psc:{pid}:1", f"qa:pb:{pid}"]
    picked = tap(client, context, f"qa:psc:{pid}:1")
    assert picked.edits[-1].text == "<b>Add RM 18.40</b> · Transport · DBS Everyday?"
    assert button_data(picked.edits[-1].markup)[0] == f"qa:pa:{pid}"
    assert client.created == []

    tap(client, context, f"qa:pa:{pid}")

    assert client.created[0]["category_id"] == "cat-tra"


def test_change_account_then_add_sends_the_picked_account():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")

    picker = tap(client, context, f"qa:pk:{pid}")
    assert button_data(picker.markups[-1]) == [f"qa:psk:{pid}:0", f"qa:psk:{pid}:1", f"qa:pb:{pid}"]
    picked = tap(client, context, f"qa:psk:{pid}:1")
    assert picked.edits[-1].text.startswith("<b>Add US$18.40</b> · Uncategorized · Wise?")
    assert "not converted" not in picked.edits[-1].text
    assert client.created == []

    tap(client, context, f"qa:pa:{pid}")

    assert client.created[0]["account_id"] == "acc-usd"


def test_back_returns_to_the_preview():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")
    query = tap(client, context, f"qa:pb:{pid}")
    assert query.edits[-1].text == "<b>Add RM 18.40</b> · Uncategorized · DBS Everyday?"


def test_pending_entry_expires_after_ten_minutes(monkeypatch):
    client = FakeUserClient()
    clock = [1000.0]
    monkeypatch.setattr(quick_add, "_now", lambda: clock[0])
    context, _, pid = preview(client, "coffee 18.40")

    clock[0] += quick_add.PENDING_TTL_SECONDS + 1
    query = tap(client, context, f"qa:pa:{pid}")

    assert query.edits[-1].text == "This entry expired — send it again."
    assert client.created == []
    assert pid not in context.user_data[quick_add.PENDING_KEY]


def test_change_buttons_also_respect_expiry(monkeypatch):
    client = FakeUserClient()
    clock = [1000.0]
    monkeypatch.setattr(quick_add, "_now", lambda: clock[0])
    context, _, pid = preview(client, "coffee 18.40")
    clock[0] += quick_add.PENDING_TTL_SECONDS + 1
    query = tap(client, context, f"qa:pc:{pid}")
    assert query.edits[-1].text == quick_add.EXPIRED_MESSAGE


def test_failed_create_keeps_the_entry_retryable():
    client = FakeUserClient()
    context, _, pid = preview(client, "coffee 18.40")

    client.fail_create = True
    failed = tap(client, context, f"qa:pa:{pid}")
    assert failed.alerts == ["Failed to record that. Please try again."]
    assert failed.edits == []  # the preview and its buttons stay
    assert pid in context.user_data[quick_add.PENDING_KEY]

    client.fail_create = False
    tap(client, context, f"qa:pa:{pid}")
    assert len(client.created) == 1


def test_unknown_pending_id_is_refused():
    client = FakeUserClient()
    query = tap(client, make_context(), "qa:pa:nope")
    assert client.created == []
    assert query.alerts == [quick_add.EXPIRED_MESSAGE]


def test_pending_entries_are_capped():
    context = make_context()
    for i in range(quick_add.MAX_PENDING + 5):
        quick_add._store_pending(context, {"categories": [], "n": i})
    assert len(context.user_data[quick_add.PENDING_KEY]) == quick_add.MAX_PENDING


def test_no_default_account_falls_back_to_asking_for_the_account():
    client = FakeUserClient(default_account_id=None)  # two spending accounts, no saved default
    update, message = make_update("coffee 18.40")
    context = make_context()

    state = run(quick_add_handler(client)(update, context))

    assert state == ACCOUNT
    assert client.created == []
    assert message.replies[-1].text == "Which account was it from?"
    txn = context.user_data[TXN_KEY]
    assert (txn["amount"], txn["description"], txn["type"]) == (1840, "coffee", "expense")


def test_a_single_spending_account_needs_no_default():
    client = FakeUserClient(accounts=[ACCOUNTS[0], ACCOUNTS[2]], default_account_id=None)
    context, _, pid = preview(client, "coffee 18.40")

    assert client.created == []
    tap(client, context, f"qa:pa:{pid}")
    assert client.created[0]["account_id"] == "acc-dbs"


@pytest.mark.parametrize("text,fragment", [
    ("hello there", "couldn't find an amount"),
    ("18.40", "What was it for"),
    ("0 coffee", "greater than zero"),
])
def test_unusable_text_is_answered_without_creating_anything(text, fragment):
    client = FakeUserClient()
    update, message = make_update(text)

    state = run(quick_add_handler(client)(update, make_context()))

    assert state == ConversationHandler.END
    assert client.created == []
    assert fragment in message.replies[-1].text


def test_same_update_is_never_previewed_twice():
    client = FakeUserClient()
    handler = quick_add_handler(client)
    context = make_context()
    update, message = make_update("coffee 18.40", update_id=42)

    run(handler(update, context))
    run(handler(update, context))

    assert len(message.replies) == 1
    assert len(context.user_data[quick_add.PENDING_KEY]) == 1


def test_text_while_another_entry_is_in_progress_does_not_create_a_second_one():
    client = FakeUserClient()
    update, message = make_update("coffee 18.40")
    context = make_context({TXN_KEY: {"type": "income"}})

    state = run(quick_add_handler(client)(update, context))

    assert state == ConversationHandler.END
    assert client.created == []
    assert "middle of another entry" in message.replies[-1].text
    assert context.user_data[TXN_KEY] == {"type": "income"}  # untouched
    assert quick_add.PENDING_KEY not in context.user_data


def test_quick_add_is_an_entry_point_of_the_expense_conversation_only():
    api = FakeAPI(FakeUserClient())
    extra = quick_add.quick_add_entry_point(api)
    expense = create_transaction_conversation(api, "expense", extra_entry_points=[extra])
    income = create_transaction_conversation(api, "income")
    assert extra in expense.entry_points
    assert extra not in income.entry_points
    assert len(income.entry_points) == 1


# --- quick-add buttons --------------------------------------------------------

class FakeQuery:
    def __init__(self, data, text="Added RM 18.40 · Dining · DBS Everyday"):
        self.data = data
        self.from_user = SimpleNamespace(id=7)
        self.message = SimpleNamespace(text=text)
        self.edits = []
        self.markups = []
        self.alerts = []

    async def answer(self, text=None, show_alert=False):
        if text:
            self.alerts.append(text)

    async def edit_message_text(self, text, reply_markup=None, parse_mode=None):
        self.edits.append(SimpleNamespace(text=text, markup=reply_markup))

    async def edit_message_reply_markup(self, reply_markup=None):
        self.markups.append(reply_markup)


def press(client, data):
    handler = quick_add.quick_add_callbacks(FakeAPI(client)).callback
    query = FakeQuery(data)
    run(handler(SimpleNamespace(callback_query=query), make_context()))
    return query


def test_undo_button_deletes_and_strikes_the_line():
    client = FakeUserClient()
    query = press(client, "qa:u:tx-9")
    assert client.deleted == ["tx-9"]
    assert query.edits[-1].text.startswith("↩️ Undone.")
    assert query.edits[-1].markup is None


def test_change_category_button_lists_expense_categories_only():
    client = FakeUserClient()
    query = press(client, "qa:c:tx-9")
    data = button_data(query.markups[-1])
    assert data == ["qa:sc:tx-9:0", "qa:sc:tx-9:1", "qa:b:tx-9"]  # Dining, Transport; no Salary


def test_picking_a_category_updates_and_rerenders():
    client = FakeUserClient()
    query = press(client, "qa:sc:tx-9:1")
    assert client.updated == [("tx-9", {"category_id": "cat-tra"})]
    assert query.edits[-1].text == "<b>Updated RM 18.40</b> · Transport · DBS Everyday"
    assert button_data(query.edits[-1].markup) == ["qa:c:tx-9", "qa:a:tx-9", "qa:u:tx-9"]


def test_change_account_lists_spending_accounts_and_updates():
    client = FakeUserClient()
    query = press(client, "qa:a:tx-9")
    assert button_data(query.markups[-1]) == ["qa:sa:tx-9:0", "qa:sa:tx-9:1", "qa:b:tx-9"]  # no investment account
    query = press(client, "qa:sa:tx-9:1")
    assert client.updated[-1] == ("tx-9", {"account_id": "acc-usd"})
    assert query.edits[-1].text == "<b>Updated US$18.40</b> · Uncategorized · Wise"


def test_stale_picker_index_is_refused():
    client = FakeUserClient()
    query = press(client, "qa:sc:tx-9:99")
    assert client.updated == []
    assert query.alerts


# --- 6.2 /undo ----------------------------------------------------------------

NOW = datetime(2026, 11, 9, 12, 0, 0, tzinfo=timezone.utc)


def txn(txn_id, source="telegram", minutes_ago=1, amount=1840, category="Dining"):
    created = NOW - timedelta(minutes=minutes_ago)
    return {
        "id": txn_id, "source": source, "amount": amount, "type": "expense",
        "created_at": created.strftime("%Y-%m-%dT%H:%M:%S.%f") + "123Z",  # Go emits nanoseconds
        "account": {"name": "DBS Everyday", "currency": "MYR"},
        "category": {"name": category} if category else None,
    }


def test_recent_bot_transactions_filters_source_and_window():
    rows = [
        txn("web", source="web", minutes_ago=1),
        txn("old", minutes_ago=11),
        txn("older-bot", minutes_ago=5),
        txn("newest-bot", minutes_ago=2),
        txn("mcp", source="mcp", minutes_ago=1),
    ]
    found = recent_bot_transactions(rows, now=NOW)
    assert [t["id"] for t in found] == ["newest-bot", "older-bot"]


def test_parse_timestamp_handles_go_formats():
    assert parse_timestamp("2026-11-09T12:00:00.123456789Z") == datetime(
        2026, 11, 9, 12, 0, 0, 123456, tzinfo=timezone.utc)
    assert parse_timestamp("2026-11-09T20:00:00+08:00") == datetime(2026, 11, 9, 12, tzinfo=timezone.utc)
    assert parse_timestamp("nope") is None and parse_timestamp(None) is None


def test_describe_transaction():
    assert describe_transaction(txn("a")) == "RM 18.40 · Dining · DBS Everyday"
    assert describe_transaction(txn("a", category=None)) == "RM 18.40 · Uncategorized · DBS Everyday"


def now_utc_txn(txn_id, minutes_ago, source="telegram"):
    row = txn(txn_id, source=source, minutes_ago=0)
    created = datetime.now(timezone.utc) - timedelta(minutes=minutes_ago)
    row["created_at"] = created.isoformat().replace("+00:00", "Z")
    return row


def test_undo_deletes_the_newest_bot_entry():
    client = FakeUserClient(transactions=[
        now_utc_txn("web-1", 1, source="web"),
        now_utc_txn("bot-new", 2),
        now_utc_txn("bot-old", 6),
    ])
    update, message = make_update("/undo")

    run(undo.handle(FakeAPI(client))(update, make_context()))

    assert client.deleted == ["bot-new"]
    assert message.replies[-1].text == "↩️ Removed RM 18.40 · Dining · DBS Everyday"


def test_undo_ignores_entries_older_than_ten_minutes_and_other_sources():
    client = FakeUserClient(transactions=[now_utc_txn("bot-old", 11), now_utc_txn("web-1", 1, source="web")])
    update, message = make_update("/undo")

    run(undo.handle(FakeAPI(client))(update, make_context()))

    assert client.deleted == []
    assert "Nothing to undo" in message.replies[-1].text


# --- 6.4 /spent ---------------------------------------------------------------

SPENDING = {
    "currency": "MYR", "total_spent": 100000, "fx_missing": False, "unconverted": [],
    "items": [
        {"category_id": "c2", "category_name": "Transport", "category_icon": "", "total": 20000},
        {"category_id": "c1", "category_name": "Dining", "category_icon": "🍜", "total": 70000},
        {"category_id": None, "category_name": "Uncategorized", "category_icon": "", "total": 10000},
    ],
}


def test_month_bounds():
    assert spent.month_bounds(datetime(2026, 2, 14, 9, tzinfo=timezone.utc)) == (
        "2026-02-01T00:00:00Z", "2026-02-28T23:59:59Z")
    assert spent.month_bounds(datetime(2026, 12, 31, tzinfo=timezone.utc))[1] == "2026-12-31T23:59:59Z"


def test_spent_message_sorts_by_amount_and_shows_base_currency():
    text = spent.format_spent_message(SPENDING, "November 2026", "MYR")
    assert text.splitlines()[0] == "💸 <b>Spent in November 2026</b>"
    assert "<b>Total:</b> RM 1,000.00" in text
    assert text.index("Dining") < text.index("Transport") < text.index("Uncategorized")
    assert "🍜 Dining: RM 700.00 (70%)" in text
    assert "Left out" not in text and "exchange rate" not in text


def test_spent_message_reports_unconverted_amounts():
    data = dict(SPENDING, fx_missing=True, unconverted=[{"currency": "JPY", "amount": 500000}])
    text = spent.format_spent_message(data, "November 2026", "MYR")
    assert "Left out (no exchange rate to MYR): JPY 5,000.00" in text


def test_spent_message_fx_missing_without_detail():
    text = spent.format_spent_message(dict(SPENDING, fx_missing=True), "November 2026", "MYR")
    assert "no exchange rate and are left out" in text


def test_spent_message_other_currency_and_escaping():
    data = {"currency": "USD", "total_spent": 1000,
            "items": [{"category_name": "R&D <x>", "category_icon": "", "total": 1000}]}
    text = spent.format_spent_message(data, "November 2026", "USD")
    assert "US$10.00" in text and "R&amp;D &lt;x&gt;" in text


def test_spent_message_empty_and_overflow():
    empty = spent.format_spent_message({"currency": "MYR", "total_spent": 0, "items": []}, "November 2026", "MYR")
    assert "Nothing spent yet" in empty
    many = {"currency": "MYR", "total_spent": 1500,
            "items": [{"category_name": f"C{i}", "total": 100} for i in range(15)]}
    text = spent.format_spent_message(many, "November 2026", "MYR")
    assert "…and 3 more: RM 3.00" in text


def test_spent_command_requests_the_current_month():
    client = FakeUserClient()
    client.spending = SPENDING
    update, message = make_update("/spent")

    run(spent.handle(FakeAPI(client))(update, make_context()))

    start, end = client.spending_range
    assert start.endswith("-01T00:00:00Z") and end.endswith("T23:59:59Z")
    assert "Spent in" in message.replies[-1].text


# --- 6.3 receipt photos -------------------------------------------------------

class FakeTelegramFile:
    async def download_as_bytearray(self):
        return bytearray(b"\xff\xd8\xffJPEGDATA")


class FakeBot:
    def __init__(self):
        self.requested = []

    async def get_file(self, file_id):
        self.requested.append(file_id)
        return FakeTelegramFile()


def photo_update(**extra):
    photo = [SimpleNamespace(file_id="small", file_size=1000), SimpleNamespace(file_id="big", file_size=50000)]
    return make_update(None, photo=photo, document=None, **extra)


def test_photo_attaches_to_the_entry_made_minutes_ago():
    client = FakeUserClient(transactions=[now_utc_txn("bot-1", 3), now_utc_txn("bot-0", 8)])
    update, message = photo_update()
    context = make_context()
    context.bot = FakeBot()

    run(receipts.handle(FakeAPI(client)).callback(update, context))

    assert context.bot.requested == ["big"]  # the largest photo size
    assert client.uploads == [("bot-1", b"\xff\xd8\xffJPEGDATA", "receipt.jpg", "image/jpeg")]
    assert message.replies[-1].text == "📎 Receipt attached to RM 18.40 · Dining · DBS Everyday"


def test_pdf_document_keeps_its_name_and_type():
    client = FakeUserClient(transactions=[now_utc_txn("bot-1", 3)])
    document = SimpleNamespace(file_id="doc", file_name="invoice.pdf", mime_type="application/pdf", file_size=2000)
    update, _ = make_update(None, photo=None, document=document)
    context = make_context()
    context.bot = FakeBot()

    run(receipts.handle(FakeAPI(client)).callback(update, context))

    assert client.uploads[0][0::2] == ("bot-1", "invoice.pdf")
    assert client.uploads[0][3] == "application/pdf"


def test_photo_without_a_recent_entry_asks_which_transaction():
    client = FakeUserClient(transactions=[
        now_utc_txn("web-1", 1, source="web"), now_utc_txn("bot-old", 30), now_utc_txn("web-2", 90, source="web"),
    ])
    update, message = photo_update()
    context = make_context()
    context.bot = FakeBot()

    run(receipts.handle(FakeAPI(client)).callback(update, context))

    assert client.uploads == []
    reply = message.replies[-1]
    assert reply.text == "Which transaction is this receipt for?"
    assert button_data(reply.markup) == ["rc:web-1", "rc:bot-old", "rc:web-2", "rc:x"]
    assert context.user_data[receipts.PENDING_KEY]["file_id"] == "big"

    # Tapping one attaches the waiting receipt to it.
    query = FakeQuery("rc:bot-old")
    run(receipts.callbacks(FakeAPI(client)).callback(SimpleNamespace(callback_query=query), context))
    assert client.uploads[0][0] == "bot-old"
    assert query.edits[-1].text == "📎 Receipt attached."
    assert receipts.PENDING_KEY not in context.user_data


def test_picker_cancel_drops_the_receipt():
    client = FakeUserClient()
    context = make_context({receipts.PENDING_KEY: {"file_id": "x"}})
    query = FakeQuery("rc:x")
    run(receipts.callbacks(FakeAPI(client)).callback(SimpleNamespace(callback_query=query), context))
    assert client.uploads == [] and receipts.PENDING_KEY not in context.user_data


def test_unsupported_and_oversized_files_are_refused_before_any_api_call():
    client = FakeUserClient(transactions=[now_utc_txn("bot-1", 3)])
    gif = SimpleNamespace(file_id="g", file_name="a.gif", mime_type="image/gif", file_size=10)
    update, message = make_update(None, photo=None, document=gif)
    run(receipts.handle(FakeAPI(client)).callback(update, make_context()))
    assert "isn't supported" in message.replies[-1].text

    huge = SimpleNamespace(file_id="h", file_name="a.pdf", mime_type="application/pdf", file_size=11 * 1024 * 1024)
    update, message = make_update(None, photo=None, document=huge)
    run(receipts.handle(FakeAPI(client)).callback(update, make_context()))
    assert "too large" in message.replies[-1].text
    assert client.uploads == []


def test_attach_error_messages():
    def status(code):
        return httpx.HTTPStatusError("x", request=httpx.Request("POST", "http://a"),
                                     response=httpx.Response(code))
    assert "too large" in receipts.attach_error_message(status(413))
    assert "isn't supported" in receipts.attach_error_message(status(415))
    assert "maximum number" in receipts.attach_error_message(status(409))
    assert "no longer exists" in receipts.attach_error_message(status(404))
    assert "Couldn't attach" in receipts.attach_error_message(httpx.ConnectError("down"))
