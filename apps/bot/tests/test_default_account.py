import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.transaction_flow import _get_default_account  # noqa: E402

ACCOUNTS = [
    {"id": "inv", "type": "investment", "is_active": True},
    {"id": "cash", "type": "cash", "is_active": True},
    {"id": "card", "type": "credit_card", "is_active": True},
    {"id": "old", "type": "cash", "is_active": False},
]


def test_falls_back_to_first_active_spending_account():
    assert _get_default_account(ACCOUNTS)["id"] == "cash"


def test_uses_preferred_account():
    assert _get_default_account(ACCOUNTS, "card")["id"] == "card"


def test_ignores_unusable_preferred_account():
    assert _get_default_account(ACCOUNTS, "old")["id"] == "cash"
    assert _get_default_account(ACCOUNTS, "inv")["id"] == "cash"
    assert _get_default_account(ACCOUNTS, "deleted")["id"] == "cash"
