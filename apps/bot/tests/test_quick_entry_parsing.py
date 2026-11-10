import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from utils.parsing import currency_matches_hint, parse_quick_entry  # noqa: E402


def _parsed(text):
    entry = parse_quick_entry(text)
    return None if entry is None else (entry.amount, entry.description)


@pytest.mark.parametrize("text,expected", [
    # amount first
    ("18.40 coffee", (1840, "coffee")),
    ("50 Lunch with Sam", (5000, "Lunch with Sam")),
    ("5 coffee", (500, "coffee")),
    # amount last
    ("coffee 18.40", (1840, "coffee")),
    ("Lunch with Sam 50", (5000, "Lunch with Sam")),
    ("lunch 2 x 5.50", (550, "lunch 2 x")),
    # one decimal
    ("coffee 4.5", (450, "coffee")),
    # thousands commas
    ("rent 1,200", (120000, "rent")),
    ("1,234,567.89 yacht", (123456789, "yacht")),
    ("1,200.50 laptop", (120050, "laptop")),
    # emoji in the description
    ("☕ coffee 18.40", (1840, "☕ coffee")),
    ("18.40 ☕", (1840, "☕")),
    ("coffee ☕ 18.40", (1840, "coffee ☕")),
    # whitespace
    ("   coffee    18.40  ", (1840, "coffee")),
    # amount only
    ("18.40", (1840, "")),
    # a leading amount wins over a trailing number
    ("2 coffees 18.40", (200, "coffees 18.40")),
])
def test_amounts_and_descriptions(text, expected):
    assert _parsed(text) == expected


@pytest.mark.parametrize("text,expected,hint", [
    ("RM18.40 coffee", (1840, "coffee"), {"MYR"}),
    ("RM 18.40 coffee", (1840, "coffee"), {"MYR"}),
    ("coffee RM18.40", (1840, "coffee"), {"MYR"}),
    ("coffee RM 18.40", (1840, "coffee"), {"MYR"}),
    ("coffee 18.40 rm", (1840, "coffee"), {"MYR"}),
    ("18.40 myr coffee", (1840, "coffee"), {"MYR"}),
    ("coffee 18.40 MYR", (1840, "coffee"), {"MYR"}),
    ("USD 20 lunch", (2000, "lunch"), {"USD"}),
    ("lunch 20 usd", (2000, "lunch"), {"USD"}),
    ("US$20 lunch", (2000, "lunch"), {"USD"}),
    ("S$5 kopi", (500, "kopi"), {"SGD"}),
    ("£3.50 tea", (350, "tea"), {"GBP"}),
    ("tea €3.50", (350, "tea"), {"EUR"}),
    ("$5 lunch", (500, "lunch"), {"USD", "SGD", "AUD", "HKD", "CAD", "NZD"}),
])
def test_currency_prefix_and_suffix_are_stripped_and_reported(text, expected, hint):
    entry = parse_quick_entry(text)
    assert (entry.amount, entry.description) == expected
    assert entry.currency_hint == frozenset(hint)


def test_no_currency_typed_means_no_hint():
    assert parse_quick_entry("coffee 18.40").currency_hint is None
    assert currency_matches_hint(None, "MYR")


def test_currency_hint_matching():
    assert currency_matches_hint(frozenset({"MYR"}), "myr")
    assert not currency_matches_hint(frozenset({"USD"}), "MYR")
    # '$' is ambiguous: any dollar currency matches, ringgit does not.
    dollar = parse_quick_entry("$5 lunch").currency_hint
    assert currency_matches_hint(dollar, "SGD")
    assert not currency_matches_hint(dollar, "MYR")


def test_amount_glued_to_description_still_parses():
    assert _parsed("50Coffee") == (5000, "Coffee")


def test_currency_words_do_not_eat_the_description():
    # 'rmb' and 'myrtle' are not currency words.
    assert _parsed("5 rmb stall") == (500, "rmb stall")
    assert _parsed("5 myrtle") == (500, "myrtle")
    assert parse_quick_entry("5 myrtle").currency_hint is None


def test_zero_amount_is_parsed_but_zero():
    assert _parsed("0 nothing") == (0, "nothing")
    assert _parsed("coffee 0") == (0, "coffee")


@pytest.mark.parametrize("text", [
    "", "   ", "hello", "coffee", "☕", "coffee .50", "coffee 50.", "19.999 coffee",
    "coffee 19.999", "1.2.3 coffee", "coffee 1.2.3", "12,34 coffee", "-5 coffee",
    "coffee -5", "coffee five", "RM coffee",
])
def test_garbage_is_rejected(text):
    assert parse_quick_entry(text) is None
