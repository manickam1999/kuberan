from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, get_base_currency, resolve_linked_user
from utils.formatting import esc, format_currency
import logging

logger = logging.getLogger(__name__)

MAX_LISTED_COUNTERPARTIES = 10

KIND_LABELS = {
    "split": "Split",
    "on_behalf": "Paid for someone",
    "claim": "Claim",
    "loan": "Loan",
    "deposit": "Deposit",
    "refund": "Refund",
}


def format_owed_message(summary: dict, base_currency: str) -> str:
    """Build the /owed message (HTML) from a receivables summary payload.

    The total is in the summary's own currency (the API's converted base
    currency); per-person and per-type amounts are in the base currency. When
    the open items span several currencies, the native per-currency totals are
    listed too, and an FX note is shown if some amounts had no rate.
    """
    total = summary.get('total_outstanding') or 0
    total_currency = summary.get('currency') or base_currency
    by_counterparty = summary.get('by_counterparty') or []
    by_currency = [c for c in (summary.get('by_currency') or []) if c.get('outstanding', 0)]

    if total <= 0 and not by_counterparty:
        return "🎉 Nobody owes you anything!"

    message = "🤝 <b>Owed to You</b>\n\n"
    message += f"<b>Total:</b> {esc(format_currency(total, total_currency))}\n"

    if len(by_currency) > 1:
        parts = [
            esc(format_currency(c.get('outstanding', 0), c.get('currency') or base_currency))
            for c in by_currency
        ]
        message += f"<b>By currency:</b> {' + '.join(parts)}\n"

    if summary.get('fx_missing'):
        message += "<i>Some amounts have no exchange rate and are left out of the total.</i>\n"
    message += "\n"

    if by_counterparty:
        message += "<b>By person</b>\n"
        for cp in by_counterparty[:MAX_LISTED_COUNTERPARTIES]:
            name = esc(cp.get('counterparty_name') or 'Unknown')
            count = cp.get('open_count', 0)
            noun = "item" if count == 1 else "items"
            message += (
                f"• {name}: {esc(format_currency(cp.get('outstanding', 0), base_currency))} "
                f"({count} open {noun})\n"
            )
        extra = len(by_counterparty) - MAX_LISTED_COUNTERPARTIES
        if extra > 0:
            message += f"…and {extra} more\n"

    kinds = [k for k in (summary.get('by_kind') or []) if k.get('outstanding', 0) > 0]
    if kinds:
        message += "\n<b>By type</b>\n"
        for k in kinds:
            kind = k.get('kind', '')
            label = KIND_LABELS.get(kind, kind.replace('_', ' ').title())
            message += (
                f"• {esc(label)}: {esc(format_currency(k.get('outstanding', 0), base_currency))}\n"
            )

    return message.rstrip("\n")


def handle(api_client: KuberanAPIClient):
    async def owed_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])
            summary = await user_client.get_receivables_summary()
            base_currency = await get_base_currency(user_client)

            await update.message.reply_text(
                format_owed_message(summary, base_currency), parse_mode=ParseMode.HTML
            )

        except Exception as e:
            logger.error("Failed to fetch receivables summary: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch what you're owed. Please try again later."
            )

    return owed_command
