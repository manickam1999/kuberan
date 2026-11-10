"""/spent: this month's spending by category, in the base currency."""
import calendar
import logging
from datetime import datetime, timezone
from typing import Any, Dict, Optional, Tuple

import httpx
from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes

from api_client import KuberanAPIClient
from handlers.common import describe_error, get_base_currency, resolve_linked_user
from utils.formatting import display_icon, esc, format_currency

logger = logging.getLogger(__name__)

MAX_LISTED_CATEGORIES = 12


def month_bounds(now: Optional[datetime] = None) -> Tuple[str, str]:
    """RFC 3339 start and end of the current (UTC) month, as the API expects."""
    now = now or datetime.now(timezone.utc)
    last_day = calendar.monthrange(now.year, now.month)[1]
    return (
        f"{now.year:04d}-{now.month:02d}-01T00:00:00Z",
        f"{now.year:04d}-{now.month:02d}-{last_day:02d}T23:59:59Z",
    )


def format_spent_message(data: Dict[str, Any], month_label: str, base_currency: str) -> str:
    """Build the /spent message (HTML) from a spending-by-category payload.

    Amounts are in ``data['currency']`` (the base currency, already converted
    by the API). Spending without an exchange rate is left out of the totals;
    ``fx_missing`` and ``unconverted`` say so.
    """
    currency = data.get("currency") or base_currency
    total = data.get("total_spent") or 0
    items = sorted(data.get("items") or [], key=lambda i: i.get("total") or 0, reverse=True)
    items = [i for i in items if (i.get("total") or 0) > 0]
    unconverted = [u for u in (data.get("unconverted") or []) if u.get("amount")]

    if not items and not unconverted and not data.get("fx_missing"):
        return f"💸 <b>Spent in {esc(month_label)}</b>\n\nNothing spent yet this month."

    message = f"💸 <b>Spent in {esc(month_label)}</b>\n\n"
    message += f"<b>Total:</b> {esc(format_currency(total, currency))}\n\n"
    for item in items[:MAX_LISTED_CATEGORIES]:
        amount = item.get("total") or 0
        name = item.get("category_name") or "Uncategorized"
        icon = display_icon(item.get("category_icon", ""))
        label = f"{icon} {name}".strip() if icon else name
        share = f" ({amount * 100 // total}%)" if total > 0 else ""
        message += f"{esc(label)}: {esc(format_currency(amount, currency))}{share}\n"
    rest = items[MAX_LISTED_CATEGORIES:]
    if rest:
        message += (
            f"…and {len(rest)} more: "
            f"{esc(format_currency(sum(i.get('total') or 0 for i in rest), currency))}\n"
        )

    if unconverted:
        left_out = ", ".join(format_currency(u["amount"], u["currency"]) for u in unconverted)
        message += (
            f"\n<i>Left out (no exchange rate to {esc(currency)}): {esc(left_out)}</i>"
        )
    elif data.get("fx_missing"):
        message += "\n<i>Some amounts have no exchange rate and are left out.</i>"
    return message.rstrip()


def handle(api_client: KuberanAPIClient):
    async def spent_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data["auth_token"])
            now = datetime.now(timezone.utc)
            from_date, to_date = month_bounds(now)
            data = await user_client.get_spending_by_category(from_date, to_date)
            base_currency = data.get("currency") or await get_base_currency(user_client)
            await update.message.reply_text(
                format_spent_message(data, now.strftime("%B %Y"), base_currency),
                parse_mode=ParseMode.HTML,
            )
        except httpx.HTTPError as e:
            logger.error("Failed to fetch spending: %s", describe_error(e))
            await update.message.reply_text("❌ Failed to fetch spending. Please try again later.")

    return spent_command
