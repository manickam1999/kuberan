import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.owed import format_owed_message  # noqa: E402


def test_empty_state():
    msg = format_owed_message(
        {"total_outstanding": 0, "by_kind": [], "by_counterparty": []}, "MYR")
    assert msg == "🎉 Nobody owes you anything!"


def test_full_summary():
    summary = {
        "total_outstanding": 15050,
        "by_kind": [
            {"kind": "split", "outstanding": 10000, "open_count": 3},
            {"kind": "on_behalf", "outstanding": 5050, "open_count": 1},
            {"kind": "loan", "outstanding": 0, "open_count": 0},
        ],
        "by_counterparty": [
            {"counterparty_key": "alice", "counterparty_name": "Alice",
             "outstanding": 12050, "open_count": 3, "oldest_date": "2026-09-01"},
            {"counterparty_key": "bob_x", "counterparty_name": "Bob_X <&>",
             "outstanding": 3000, "open_count": 1, "oldest_date": "2026-10-01"},
        ],
    }
    msg = format_owed_message({**summary, "currency": "MYR"}, "MYR")
    assert "<b>Total:</b> RM 150.50" in msg
    assert "By currency" not in msg
    assert "• Alice: RM 120.50 (3 open items)" in msg
    assert "• Bob_X &lt;&amp;&gt;: RM 30.00 (1 open item)" in msg
    assert "Split: RM 100.00" in msg
    assert "Paid for someone: RM 50.50" in msg
    assert "Loan" not in msg


def test_owed_with_by_currency_and_fx_missing():
    summary = {
        "total_outstanding": 20000,
        "currency": "MYR",
        "fx_missing": True,
        "by_currency": [
            {"currency": "MYR", "outstanding": 10000},
            {"currency": "USD", "outstanding": 2000},
            {"currency": "SGD", "outstanding": 0},
        ],
        "by_kind": [{"kind": "loan", "outstanding": 20000, "open_count": 2}],
        "by_counterparty": [
            {"counterparty_name": "Carol", "outstanding": 20000, "open_count": 2},
        ],
    }
    msg = format_owed_message(summary, "MYR")
    assert "<b>Total:</b> RM 200.00\n<b>By currency:</b> RM 100.00 + US$20.00\n" in msg
    assert "no exchange rate" in msg
    assert "• Carol: RM 200.00 (2 open items)" in msg


def test_owed_total_uses_summary_currency_when_present():
    summary = {"total_outstanding": 5000, "currency": "USD", "by_counterparty": [],
               "by_kind": []}
    msg = format_owed_message(summary, "MYR")
    assert "<b>Total:</b> US$50.00" in msg


def test_top_10_limit():
    cps = [{"counterparty_name": f"P{i}", "outstanding": 100 * (20 - i),
            "open_count": 1} for i in range(12)]
    msg = format_owed_message(
        {"total_outstanding": 99999, "by_kind": [], "by_counterparty": cps}, "USD")
    assert "P9:" in msg and "P10:" not in msg
    assert "…and 2 more" in msg
