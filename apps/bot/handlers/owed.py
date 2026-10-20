from telegram import Update
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient, UserAPIClient
from utils.formatting import format_currency
import logging

logger = logging.getLogger(__name__)

MAX_COUNTERPARTIES = 10

KIND_LABELS = {
    "split": "Split",
    "on_behalf": "Paid for someone",
    "claim": "Claim",
    "loan": "Loan",
    "deposit": "Deposit",
    "refund": "Refund",
}


def escape_markdown(text: str) -> str:
    """Escape characters that are special in Telegram legacy Markdown."""
    for ch in ("\\", "_", "*", "`", "["):
        text = text.replace(ch, "\\" + ch)
    return text


def format_owed_message(summary: dict, currency: str) -> str:
    """Build the /owed message from a receivables summary payload."""
    total = summary.get('total_outstanding') or 0
    by_counterparty = summary.get('by_counterparty') or []

    if total <= 0 and not by_counterparty:
        return "🎉 Nobody owes you anything!"

    message = "🤝 *Owed to You*\n\n"
    message += f"*Total:* {format_currency(total, currency)}\n\n"

    if by_counterparty:
        message += "*By person*\n"
        for cp in by_counterparty[:MAX_COUNTERPARTIES]:
            name = escape_markdown(cp.get('counterparty_name') or 'Unknown')
            count = cp.get('open_count', 0)
            noun = "item" if count == 1 else "items"
            message += (
                f"• {name}: {format_currency(cp.get('outstanding', 0), currency)} "
                f"({count} open {noun})\n"
            )
        extra = len(by_counterparty) - MAX_COUNTERPARTIES
        if extra > 0:
            message += f"…and {extra} more\n"

    kinds = [k for k in (summary.get('by_kind') or []) if k.get('outstanding', 0) > 0]
    if kinds:
        message += "\n*By type*\n"
        for k in kinds:
            kind = k.get('kind', '')
            label = KIND_LABELS.get(kind, kind.replace('_', ' ').title())
            message += f"• {label}: {format_currency(k.get('outstanding', 0), currency)}\n"

    return message.rstrip("\n")


def handle(api_client: KuberanAPIClient):
    async def owed_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        telegram_user_id = update.message.from_user.id

        # Resolve Telegram user to Kuberan user
        user_data = api_client.resolve_user(telegram_user_id)
        if not user_data:
            await update.message.reply_text(
                "⚠️ Your Telegram account is not linked.\n"
                "Use /start to link your account."
            )
            return

        # Record activity
        api_client.record_activity(telegram_user_id)

        try:
            user_client = UserAPIClient(api_client.base_url, user_data['auth_token'])
            summary = user_client.get_receivables_summary()
            currency = user_data.get('default_currency', 'MYR')

            await update.message.reply_text(
                format_owed_message(summary, currency), parse_mode='Markdown'
            )

        except Exception as e:
            logger.error(f"Failed to fetch receivables summary: {e}")
            await update.message.reply_text(
                "❌ Failed to fetch what you're owed. Please try again later."
            )

    return owed_command
