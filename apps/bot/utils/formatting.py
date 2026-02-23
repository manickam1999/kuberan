def format_currency(amount_cents: int, currency: str = "MYR") -> str:
    """
    Format amount in cents to currency string.

    Args:
        amount_cents: Amount in cents (e.g., 1050 for $10.50)
        currency: Currency code (default: USD)

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

    symbol = currency_symbols.get(currency, currency + " ")

    return f"{symbol}{amount_dollars:,.2f}"


def format_account_type(account_type: str) -> str:
    """Format account type for display"""
    type_map = {
        "cash": "💵 Cash",
        "investment": "📈 Investment",
        "credit_card": "💳 Credit Card"
    }
    return type_map.get(account_type, account_type.title())


def format_percentage(value: float) -> str:
    """Format percentage value"""
    return f"{value:.1f}%"
