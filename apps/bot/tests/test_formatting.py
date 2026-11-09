import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.balance import format_balance_message  # noqa: E402
from handlers.categories import format_categories_message  # noqa: E402
from handlers.transaction_flow import _format_confirm_message  # noqa: E402
from utils.formatting import esc, format_currency  # noqa: E402
import pytest  # noqa: E402


def test_esc_escapes_html():
    assert esc("<b>&") == "&lt;b&gt;&amp;"
    assert esc(None) == ""
    assert esc(5) == "5"


def test_balance_message_escapes_account_names():
    msg = format_balance_message(
        [{"name": "<a href='x'>Pwn</a>", "balance": 1050, "currency": "USD", "type": "cash",
          "balance_base": 1050}],
        "USD",
    )
    assert "<a href" not in msg
    assert "&lt;a href='x'&gt;Pwn&lt;/a&gt;" in msg
    assert "<b>Total (USD):</b> US$10.50" in msg


def test_format_currency_known_symbols():
    assert format_currency(123450, "MYR") == "RM 1,234.50"
    assert format_currency(1050, "USD") == "US$10.50"
    assert format_currency(100, "SGD") == "S$1.00"
    assert format_currency(100, "GBP") == "£1.00"
    assert format_currency(100, "EUR") == "€1.00"
    assert format_currency(100, "AUD") == "A$1.00"
    assert format_currency(100, "HKD") == "HK$1.00"


def test_format_currency_unknown_code_uses_prefix():
    assert format_currency(123456789, "JPY") == "JPY 1,234,567.89"


def test_format_currency_negative_and_zero():
    assert format_currency(-123450, "MYR") == "-RM 1,234.50"
    assert format_currency(-5, "USD") == "-US$0.05"
    assert format_currency(0, "USD") == "US$0.00"


def test_format_currency_requires_currency():
    with pytest.raises(TypeError):
        format_currency(100)  # type: ignore[call-arg]


def test_balance_total_uses_balance_base_and_lists_unconverted_accounts():
    accounts = [
        {"name": "Main", "type": "cash", "balance": 10000, "currency": "MYR", "balance_base": 10000},
        {"name": "Travel", "type": "cash", "balance": 5000, "currency": "USD", "balance_base": 22500},
        {"name": "Crypto <x>", "type": "investment", "balance": 700, "currency": "SGD"},
        {"name": "Old", "type": "cash", "balance": 999, "currency": "USD",
         "balance_base": 999, "is_active": False},
    ]
    msg = format_balance_message(accounts, "MYR")
    assert "Balance: RM 100.00" in msg
    assert "Balance: US$50.00" in msg
    assert "Balance: S$7.00" in msg
    # 100.00 + 225.00 MYR (balance_base); the raw 50 USD and 7 SGD are never added in.
    assert "<b>Total (MYR):</b> RM 325.00" in msg
    assert "<b>Not included in total (no exchange rate)</b>" in msg
    assert "• Crypto &lt;x&gt;: S$7.00" in msg
    assert "Old" not in msg  # inactive accounts are skipped entirely


def test_balance_without_rates_totals_zero_in_base():
    msg = format_balance_message(
        [{"name": "Card", "type": "cash", "balance": 500, "currency": "USD"}], "MYR")
    assert "<b>Total (MYR):</b> RM 0.00" in msg
    assert "• Card: US$5.00" in msg


def test_categories_message_escapes_user_text():
    msg = format_categories_message([
        {"id": "p", "name": "Food & Drink", "type": "expense", "description": "<i>x"},
        {"id": "c", "name": "Snacks_*", "type": "expense", "parent_id": "p"},
    ])
    assert "<b>Food &amp; Drink</b> - <i>&lt;i&gt;x</i>" in msg
    assert "  Snacks_*" in msg


def test_confirm_message_escapes_description_and_names():
    msg = _format_confirm_message({
        "type": "expense", "amount": 1999, "currency": "MYR",
        "description": "Tea <script>", "account_name": "A&B",
        "category_name": "Food", "category_icon": "",
    })
    assert msg.startswith("<b>Expense: RM 19.99</b>\n")
    assert "Tea &lt;script&gt;" in msg
    assert "Account: A&amp;B" in msg
