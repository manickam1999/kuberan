from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, get_base_currency, resolve_linked_user
from utils.formatting import esc, format_currency
from datetime import datetime
import logging

logger = logging.getLogger(__name__)


def handle(api_client: KuberanAPIClient):
    async def summary_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])

            # Get current month summary
            now = datetime.now()
            summary = await user_client.get_monthly_summary(months=1)

            if not summary:
                await update.message.reply_text(
                    "No transaction data available for this month."
                )
                return

            # Parse summary data
            item = {}

            if isinstance(summary, list) and len(summary) > 0:
                item = summary[0]
            elif isinstance(summary, dict):
                item = summary
            income = item.get('income', 0)
            expenses = item.get('expenses', 0)
            net = income - expenses

            # The month's totals are in the item's currency (the base currency).
            currency = item.get('currency') or await get_base_currency(user_client)
            month_name = now.strftime("%B %Y")

            message = f"📈 <b>Monthly Summary - {esc(month_name)}</b>\n\n"
            message += f"💰 Income: {esc(format_currency(income, currency))}\n"
            message += f"💸 Spending: {esc(format_currency(expenses, currency))}\n"
            message += "━━━━━━━━━━━━━━━━\n"

            status = "✅" if net >= 0 else "⚠️"
            message += f"{status} Net (income − spending): {esc(format_currency(net, currency))}"
            if item.get('fx_missing'):
                message += "\n<i>Some amounts have no exchange rate and are left out.</i>"

            await update.message.reply_text(message, parse_mode=ParseMode.HTML)

        except Exception as e:
            logger.error("Failed to fetch summary: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch summary. Please try again later."
            )

    return summary_command
