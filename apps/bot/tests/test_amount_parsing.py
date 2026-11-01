import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from handlers.transaction_flow import _parse_amount_description, amount_to_cents  # noqa: E402


@pytest.mark.parametrize("text,cents", [
    # Values where float(x) * 100 truncates to one cent less.
    ("19.99", 1999),
    ("4.35", 435),
    ("0.29", 29),
    ("8.20", 820),
    ("1.1", 110),
    ("50", 5000),
    ("0.01", 1),
    ("1234567.89", 123456789),
])
def test_amount_to_cents_is_exact(text, cents):
    assert amount_to_cents(text) == cents


def test_amount_to_cents_rejects_garbage():
    assert amount_to_cents("abc") is None
    assert amount_to_cents("NaN") is None
    assert amount_to_cents("-1") is None


@pytest.mark.parametrize("text,expected", [
    ("19.99 Coffee", (1999, "Coffee")),
    ("4.35", (435, "")),
    ("0.29 gum at the shop", (29, "gum at the shop")),
    ("  8.20   Lunch  ", (820, "Lunch")),
    ("50Coffee", (5000, "Coffee")),
    ("0 nothing", (0, "nothing")),
])
def test_parse_amount_description(text, expected):
    assert _parse_amount_description(text) == expected


@pytest.mark.parametrize("text", ["", "Coffee 5", "19.999", "1.2.3", ".5", "50."])
def test_parse_amount_description_rejects(text):
    assert _parse_amount_description(text) == (None, "")
