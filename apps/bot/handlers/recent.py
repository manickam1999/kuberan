"""Helpers for finding and describing transactions the bot recently created."""
import re
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

from utils.formatting import display_icon, format_currency

# /undo and receipt photos only look at entries this recent.
RECENT_WINDOW = timedelta(minutes=10)
# The list endpoint cannot filter by source, so the newest page is fetched and
# filtered here. Entries made in the last 10 minutes are always near the top
# (the API sorts by date, which the bot leaves at "now").
RECENT_PAGE_SIZE = 50

BOT_SOURCE = "telegram"

_FRACTION_RE = re.compile(r"\.(\d+)")


def parse_timestamp(value: Any) -> Optional[datetime]:
    """Parse an API timestamp (RFC 3339, up to 9 fractional digits) to aware UTC."""
    if not isinstance(value, str) or not value:
        return None
    # Go emits nanoseconds; fromisoformat only takes up to 6 digits.
    text = _FRACTION_RE.sub(lambda m: "." + m.group(1)[:6].ljust(6, "0"), value, count=1)
    try:
        parsed = datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def recent_bot_transactions(transactions: List[Dict[str, Any]],
                            now: Optional[datetime] = None,
                            window: timedelta = RECENT_WINDOW) -> List[Dict[str, Any]]:
    """Transactions created by the bot within ``window``, newest first."""
    now = now or datetime.now(timezone.utc)
    found = []
    for txn in transactions:
        if txn.get("source") != BOT_SOURCE:
            continue
        created = parse_timestamp(txn.get("created_at"))
        if created is None or now - created > window or created > now + timedelta(minutes=1):
            continue
        found.append((created, txn))
    found.sort(key=lambda pair: pair[0], reverse=True)
    return [txn for _, txn in found]


def describe_transaction(txn: Dict[str, Any], fallback_currency: str = "MYR") -> str:
    """Plain-text 'RM 18.40 · Dining · DBS Everyday' (callers escape it)."""
    account = txn.get("account") or {}
    category = txn.get("category") or {}
    currency = account.get("currency") or fallback_currency
    parts = [format_currency(txn.get("amount") or 0, currency)]
    cat_name = category.get("name")
    if cat_name:
        icon = display_icon(category.get("icon", ""))
        parts.append(f"{icon} {cat_name}".strip() if icon else cat_name)
    else:
        parts.append("Uncategorized")
    if account.get("name"):
        parts.append(account["name"])
    return " · ".join(parts)
