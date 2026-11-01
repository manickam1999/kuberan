from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
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


def format_owed_message(summary: dict, currency: str) -> str:
    """Build the /owed message (HTML) from a receivables summary payload."""
    total = summary.get('total_outstanding') or 0
    by_counterparty = summary.get('by_counterparty') or []

    if total <= 0 and not by_counterparty:
        return "🎉 Nobody owes you anything!"

    message = "🤝 <b>Owed to You</b>\n\n"
    message += f"<b>Total:</b> {esc(format_currency(total, currency))}\n\n"

    if by_counterparty:
        message += "<b>By person</b>\n"
        for cp in by_counterparty[:MAX_LISTED_COUNTERPARTIES]:
            name = esc(cp.get('counterparty_name') or 'Unknown')
            count = cp.get('open_count', 0)
            noun = "item" if count == 1 else "items"
            message += (
                f"• {name}: {esc(format_currency(cp.get('outstanding', 0), currency))} "
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
            message += f"• {esc(label)}: {esc(format_currency(k.get('outstanding', 0), currency))}\n"

    return message.rstrip("\n")


def handle(api_client: KuberanAPIClient):
    async def owed_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])
            summary = await user_client.get_receivables_summary()
            currency = user_data.get('default_currency', 'MYR')

            await update.message.reply_text(
                format_owed_message(summary, currency), parse_mode=ParseMode.HTML
            )

        except Exception as e:
            logger.error("Failed to fetch receivables summary: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch what you're owed. Please try again later."
            )

    return owed_command
