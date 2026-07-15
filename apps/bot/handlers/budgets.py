from telegram import Update
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient, UserAPIClient
from utils.formatting import format_currency, format_percentage
import logging

logger = logging.getLogger(__name__)

def handle(api_client: KuberanAPIClient):
    async def budgets_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
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

            # Get budgets
            budgets = user_client.get_budgets()

            if not budgets:
                await update.message.reply_text(
                    "You don't have any budgets yet.\n"
                    "Create budgets in the web app to track your spending!"
                )
                return

            # Fetch progress for all active budgets in a single batch call,
            # then index it by budget_id to avoid a per-budget HTTP request.
            progress_by_budget = {
                p['budget_id']: p
                for p in user_client.get_budgets_progress()
                if p.get('budget_id')
            }

            message = "📊 *Your Budgets*\n\n"

            for budget in budgets:
                if not budget.get('is_active', True):
                    continue

                name = budget.get('name', 'Unnamed Budget')
                amount = budget.get('amount', 0)
                period = budget.get('period', 'monthly')

                progress = progress_by_budget.get(budget['id'])
                if progress:
                    spent = progress.get('spent', 0)
                    remaining = progress.get('remaining', 0)
                    percentage = progress.get('percentage', 0)

                    status_emoji = "✅" if percentage < 80 else "⚠️" if percentage < 100 else "🚨"

                    message += f"{status_emoji} *{name}* ({period})\n"
                    message += f"Budget: {format_currency(amount)}\n"
                    message += f"Spent: {format_currency(spent)} ({format_percentage(percentage)})\n"
                    message += f"Remaining: {format_currency(remaining)}\n\n"
                else:
                    # No matching progress entry; show budget info only.
                    message += f"📋 *{name}* ({period})\n"
                    message += f"Budget: {format_currency(amount)}\n\n"

            await update.message.reply_text(message, parse_mode='Markdown')

        except Exception as e:
            logger.error(f"Failed to fetch budgets: {e}")
            await update.message.reply_text(
                "❌ Failed to fetch budgets. Please try again later."
            )

    return budgets_command
