import re
from decimal import Decimal, InvalidOperation, ROUND_HALF_UP
from typing import FrozenSet, NamedTuple, Optional


def find_account_by_name(accounts: list, account_name: Optional[str] = None) -> Optional[int]:
    """
    Find account ID by name or return first account.

    Args:
        accounts: List of account objects
        account_name: Optional account name to search for

    Returns:
        Account ID or None
    """
    if not accounts:
        return None

    if account_name:
        # Case-insensitive search
        for acc in accounts:
            if acc.get('name', '').lower() == account_name.lower():
                return acc.get('id')

    # Return first active account as default
    for acc in accounts:
        if acc.get('is_active', True):
            return acc.get('id')

    # Fallback to first account
    return accounts[0].get('id')


# --- Amount + description parsing (one-line quick add) -----------------------

def amount_to_cents(amount: str) -> Optional[int]:
    """Convert a decimal amount string (at most 2 decimals) to int cents.

    Uses Decimal, never float: float('19.99') * 100 is 1998.999..., which
    int() truncates to 1998. Thousands separators ('1,200.50') are accepted.
    """
    try:
        value = Decimal(amount.replace(",", ""))
    except InvalidOperation:
        return None
    if not value.is_finite() or value < 0:
        return None
    return int((value * 100).quantize(Decimal(1), rounding=ROUND_HALF_UP))


_DOLLAR_FAMILY = frozenset({"USD", "SGD", "AUD", "HKD", "CAD", "NZD"})

# Currency symbols/prefixes a user may type next to the amount, and the ISO
# codes each one can mean ("$" is ambiguous, so it stands for the whole family).
_SYMBOLS = {
    "rm": frozenset({"MYR"}),
    "us$": frozenset({"USD"}),
    "s$": frozenset({"SGD"}),
    "a$": frozenset({"AUD"}),
    "hk$": frozenset({"HKD"}),
    "$": _DOLLAR_FAMILY,
    "£": frozenset({"GBP"}),
    "€": frozenset({"EUR"}),
}
# Three-letter codes accepted as a currency word (an arbitrary 3-letter word is
# not: "buy 5" must keep "buy" as the description).
_CODES = frozenset({
    "MYR", "USD", "SGD", "EUR", "GBP", "AUD", "HKD", "CAD", "NZD", "CHF", "JPY",
    "CNY", "THB", "IDR", "INR", "KRW", "TWD", "PHP", "VND",
})

_SYMBOL_RE = r"(?:rm|us\$|s\$|a\$|hk\$|\$|£|€)"
_CODE_RE = r"(?:" + "|".join(sorted(c.lower() for c in _CODES)) + r")"
_CURRENCY_RE = r"(?:" + _CODE_RE + r"|" + _SYMBOL_RE + r")"
# 1,234.50 | 1234.5 | 50 (at most 2 decimals).
_NUM_RE = r"(?:\d{1,3}(?:,\d{3})+(?:\.\d{1,2})?|\d+(?:\.\d{1,2})?)"

# '[RM] 18.40 [MYR] coffee': the amount leads. The lookahead rejects '19.999',
# '1.2.3' and '12,34' instead of splitting them into an amount plus a numeric
# description. The amount may touch the description ('50Coffee').
_AMOUNT_FIRST = re.compile(
    r"^(?:(?P<pre>" + _CURRENCY_RE + r")\s*)?(?P<num>" + _NUM_RE + r")(?![\d.]|,\d)"
    r"(?:\s*(?P<suf>" + _CODE_RE + r"|rm)(?![A-Za-z0-9]))?\s*(?P<desc>.*)$",
    re.IGNORECASE | re.DOTALL,
)
# 'coffee [RM] 18.40 [MYR]': the amount closes the text and starts a token.
_AMOUNT_LAST = re.compile(
    r"^(?P<desc>.*?\S)\s+(?:(?P<pre>" + _CURRENCY_RE + r")\s*)?(?P<num>" + _NUM_RE + r")"
    r"(?:\s*(?P<suf>" + _CODE_RE + r"|rm))?$",
    re.IGNORECASE | re.DOTALL,
)


class ParsedEntry(NamedTuple):
    """A parsed 'amount + description' message."""
    amount: int  # cents; 0 is parsed but invalid (callers reject it)
    description: str
    # ISO codes the typed currency prefix/suffix could mean; None if none typed.
    currency_hint: Optional[FrozenSet[str]]


def _hint(*tokens: Optional[str]) -> Optional[FrozenSet[str]]:
    for token in tokens:
        if not token:
            continue
        key = token.lower()
        if key in _SYMBOLS:
            return _SYMBOLS[key]
        return frozenset({key.upper()})
    return None


def parse_quick_entry(text: str) -> Optional[ParsedEntry]:
    """Parse '18.40 coffee' or 'coffee 18.40' (amount first or last).

    Accepts decimals (at most 2), thousands commas ('1,200 rent'), a currency
    prefix or suffix ('RM18.40', '$5 lunch', 'lunch 5 usd') and emoji in the
    description. The amount-first form wins when both could apply ('2 coffees
    18.40' is RM 2.00). Returns None when there is no valid amount.
    """
    if not text:
        return None
    text = text.strip()
    for pattern in (_AMOUNT_FIRST, _AMOUNT_LAST):
        match = pattern.match(text)
        if not match:
            continue
        cents = amount_to_cents(match.group("num"))
        if cents is None:
            continue
        return ParsedEntry(
            cents,
            match.group("desc").strip(),
            _hint(match.group("pre"), match.group("suf")),
        )
    return None


def currency_matches_hint(hint: Optional[FrozenSet[str]], currency: str) -> bool:
    """True when no currency was typed or it is compatible with ``currency``."""
    return not hint or currency.upper() in hint


def describe_hint(hint: FrozenSet[str]) -> str:
    """Human label for a typed currency hint ('USD', or 'USD/SGD/...' for '$')."""
    return "/".join(sorted(hint))
