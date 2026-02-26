from telegram import Update
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient, UserAPIClient
from utils.formatting import format_currency
from datetime import datetime
import logging

logger = logging.getLogger(__name__)

def handle(api_client: KuberanAPIClient):
    async def summary_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
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
            # Create user-scoped client
            user_client = UserAPIClient(api_client.base_url, user_data['auth_token'])

            # Get current month summary
            now = datetime.now()
            summary = user_client.get_monthly_summary(months=1)

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

            message = f"📈 *Monthly Summary - {month_name}*\n\n"
            message += f"💰 Income: {format_currency(income, currency)}\n"
            message += f"💸 Expenses: {format_currency(expenses, currency)}\n"
            message += f"━━━━━━━━━━━━━━━━\n"

            if net >= 0:
                message += f"✅ Net: {format_currency(net, currency)}"
            else:
                message += f"⚠️ Net: {format_currency(net, currency)}"

            await update.message.reply_text(message, parse_mode='Markdown')

        except Exception as e:
            logger.error(f"Failed to fetch summary: {e}")
            await update.message.reply_text(
                "❌ Failed to fetch summary. Please try again later."
            )

    return summary_command
