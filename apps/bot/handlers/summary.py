from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
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
            income = 0
            expenses = 0

            if isinstance(summary, list) and len(summary) > 0:
                current_month_data = summary[0]
                income = current_month_data.get('income', 0)
                expenses = current_month_data.get('expenses', 0)
            elif isinstance(summary, dict):
                income = summary.get('income', 0)
                expenses = summary.get('expenses', 0)

            net = income - expenses

            currency = user_data.get('default_currency', 'MYR')
            month_name = now.strftime("%B %Y")

            message = f"📈 <b>Monthly Summary - {esc(month_name)}</b>\n\n"
            message += f"💰 Income: {esc(format_currency(income, currency))}\n"
            message += f"💸 Spending: {esc(format_currency(expenses, currency))}\n"
            message += "━━━━━━━━━━━━━━━━\n"

            status = "✅" if net >= 0 else "⚠️"
            message += f"{status} Net (income − spending): {esc(format_currency(net, currency))}"

            await update.message.reply_text(message, parse_mode=ParseMode.HTML)

        except Exception as e:
            logger.error("Failed to fetch summary: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch summary. Please try again later."
            )

    return summary_command
