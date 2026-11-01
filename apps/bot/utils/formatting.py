import html
from typing import Any


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
