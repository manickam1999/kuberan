import html
import unicodedata
from typing import Any

# Upper bound on code points in one emoji icon; matches maxEmojiRunes in
# apps/api/internal/services/category_icons.go.
_MAX_EMOJI_CODEPOINTS = 10


def is_emoji_icon(text: str) -> bool:
    """Report whether text looks like a single emoji, the way the API checks it.

    Every code point must be a symbol or an emoji joiner/modifier, with at least
    one symbol. Letters, digits, spaces and punctuation are rejected.
    """
    if not text or len(text) > _MAX_EMOJI_CODEPOINTS:
        return False
    has_symbol = False
    for ch in text:
        cp = ord(ch)
        if unicodedata.category(ch) in ("So", "Sm", "Sc", "Sk"):
            has_symbol = True
        elif cp in (0x200D, 0xFE0E, 0xFE0F) or 0xE0020 <= cp <= 0xE007F:
            continue
        else:
            return False
    return has_symbol


def display_icon(icon: str) -> str:
    """Return a category icon for Telegram text, or "" if it can't be shown.

    Categories may store a named icon (e.g. "shopping-cart", used by the web
    app) instead of an emoji; Telegram can only show the emoji ones.
    """
    return icon if is_emoji_icon(icon or "") else ""


def format_currency(amount_cents: int, currency: str = "MYR") -> str:
    """
    Format amount in cents to currency string.

    Args:
        amount_cents: Amount in cents (e.g., 1050 for $10.50)
        currency: Currency code (default: MYR)

    Returns:
        Formatted currency string (e.g., "$10.50")
    """
    amount_dollars = amount_cents / 100.0

    currency_symbols = {
        "USD": "$",
        "EUR": "€",
        "GBP": "£",
        "INR": "₹",
        "MYR": "RM",
    }

    symbol = currency_symbols.get(currency, f"{currency} ")

    return f"{symbol}{amount_dollars:,.2f}"


def format_account_type(account_type: str) -> str:
    """Format account type for display"""
    type_map = {
        "cash": "💵 Cash",
        "investment": "📈 Investment",
        "credit_card": "💳 Credit Card",
        "debt": "🏦 Debt",
    }
    return type_map.get(account_type, account_type.title())


def format_percentage(value: float) -> str:
    """Format percentage value"""
    return f"{value:.1f}%"


def esc(value: Any) -> str:
    """Escape a value for a Telegram message sent with parse_mode='HTML'.

    Every piece of user- or API-supplied text (names, descriptions, icons,
    currency codes) must go through this before it is interpolated into an
    HTML reply, otherwise a stray '<' or '&' breaks the message (Telegram
    rejects it) or injects formatting/links.
    """
    return html.escape("" if value is None else str(value), quote=False)
