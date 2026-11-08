import asyncio
import os
import sys
from types import SimpleNamespace

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.transaction_flow import (  # noqa: E402
    ACCOUNT, TXN_KEY, _build_account_keyboard, _build_confirm_keyboard,
    create_transaction_conversation,
)

ACCOUNTS = [
    {"id": "myr", "name": "Main", "type": "cash", "currency": "MYR", "is_active": True},
    {"id": "usd", "name": "IBKR Cash", "type": "cash", "currency": "USD", "is_active": True},
    {"id": "inv", "name": "Brokerage", "type": "investment", "currency": "USD", "is_active": True},
]


def _button_texts(markup):
    return [button.text for row in markup.inline_keyboard for button in row]


def test_account_buttons_show_currency_when_accounts_differ():
    texts = _button_texts(_build_account_keyboard(ACCOUNTS))
    assert "Main (MYR)" in texts
    assert "IBKR Cash (USD)" in texts
    assert "Brokerage" not in texts  # investment accounts are not spending accounts


def test_account_buttons_plain_when_single_currency():
    same = [dict(a, currency="USD") for a in ACCOUNTS]
    texts = _button_texts(_build_account_keyboard(same))
    assert "Main" in texts and "IBKR Cash" in texts


def test_change_currency_button_is_gone():
    assert "Change Currency" not in _button_texts(_build_confirm_keyboard())


class _FakeQuery:
    def __init__(self, data):
        self.data = data
        self.edited = None

    async def answer(self):
        return None

    async def edit_message_text(self, text, reply_markup=None, parse_mode=None):
        self.edited = text


def test_picking_usd_account_confirms_in_usd():
    conv = create_transaction_conversation(api_client=None, txn_type="expense")
    handler = conv.states[ACCOUNT][0].callback
    txn = {
        "type": "expense", "amount": 1000, "description": "Lunch", "currency": "MYR",
        "accounts": ACCOUNTS, "account_id": "myr", "account_name": "Main",
        "category_id": None, "category_name": None, "category_icon": "",
    }
    context = SimpleNamespace(user_data={TXN_KEY: txn})
    query = _FakeQuery("acc:usd")
    update = SimpleNamespace(callback_query=query)

    asyncio.run(handler(update, context))

    assert txn["currency"] == "USD"
    assert txn["account_name"] == "IBKR Cash"
    assert query.edited.startswith("<b>Expense: US$10.00</b>\n")
    assert "Currency: USD" in query.edited
