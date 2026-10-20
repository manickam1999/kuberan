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
            {"counterparty_key": "bob_x", "counterparty_name": "Bob_X",
             "outstanding": 3000, "open_count": 1, "oldest_date": "2026-10-01"},
        ],
    }
    msg = format_owed_message(summary, "MYR")
    assert "*Total:* RM150.50" in msg
    assert "• Alice: RM120.50 (3 open items)" in msg
    assert "• Bob\\_X: RM30.00 (1 open item)" in msg
    assert "Split: RM100.00" in msg
    assert "Paid for someone: RM50.50" in msg
    assert "Loan" not in msg


def test_top_10_limit():
    cps = [{"counterparty_name": f"P{i}", "outstanding": 100 * (20 - i),
            "open_count": 1} for i in range(12)]
    msg = format_owed_message(
        {"total_outstanding": 99999, "by_kind": [], "by_counterparty": cps}, "USD")
    assert "P9:" in msg and "P10:" not in msg
    assert "…and 2 more" in msg
