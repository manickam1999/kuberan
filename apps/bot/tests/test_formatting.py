import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.balance import format_balance_message  # noqa: E402
from handlers.categories import format_categories_message  # noqa: E402
from handlers.transaction_flow import _format_confirm_message  # noqa: E402
from utils.formatting import esc  # noqa: E402


def test_esc_escapes_html():
    assert esc("<b>&") == "&lt;b&gt;&amp;"
    assert esc(None) == ""
    assert esc(5) == "5"


def test_balance_message_escapes_account_names():
    msg = format_balance_message(
        [{"name": "<a href='x'>Pwn</a>", "balance": 1050, "currency": "USD", "type": "cash"}],
        "USD",
    )
    assert "<a href" not in msg
    assert "&lt;a href='x'&gt;Pwn&lt;/a&gt;" in msg
    assert "<b>Total:</b> $10.50" in msg


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
    assert msg.startswith("<b>Expense: RM19.99</b>\n")
    assert "Tea &lt;script&gt;" in msg
    assert "Account: A&amp;B" in msg
