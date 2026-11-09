import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.budgets import format_budgets_message  # noqa: E402


def test_budgets_formatted_in_each_budget_currency():
    budgets = [
        {"id": "b1", "name": "Groceries", "amount": 50000, "period": "monthly",
         "currency": "MYR", "is_active": True},
        {"id": "b2", "name": "Travel", "amount": 20000, "period": "monthly",
         "currency": "USD", "is_active": True},
    ]
    progress = {
        "b1": {"budget_id": "b1", "spent": 12000, "remaining": 38000,
               "percentage": 24.0, "currency": "MYR"},
        "b2": {"budget_id": "b2", "spent": 15000, "remaining": 5000,
               "percentage": 75.0, "currency": "USD"},
    }
    msg = format_budgets_message(budgets, progress, "MYR")
    assert "Budget: RM 500.00" in msg
    assert "Spent: RM 120.00 (24.0%)" in msg
    assert "Budget: US$200.00" in msg
    assert "Spent: US$150.00 (75.0%)" in msg
    assert "Remaining: US$50.00" in msg
    assert "Budget: RM" not in msg.split("Travel")[1]


def test_budget_without_progress_uses_its_own_currency():
    msg = format_budgets_message(
        [{"id": "b3", "name": "Fun", "amount": 1500, "period": "weekly", "currency": "USD"}],
        {}, "MYR")
    assert "📋 <b>Fun</b> (weekly)\nBudget: US$15.00" in msg


def test_budget_falls_back_to_base_currency_when_missing():
    msg = format_budgets_message(
        [{"id": "b4", "name": "Misc", "amount": 1000, "period": "monthly"}], {}, "SGD")
    assert "Budget: S$10.00" in msg
