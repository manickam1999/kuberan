from telegram import Update
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient, UserAPIClient
from utils.formatting import format_currency, format_account_type
import logging

logger = logging.getLogger(__name__)

def handle(api_client: KuberanAPIClient):
    async def balance_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        telegram_user_id = update.message.from_user.id

        # Resolve Telegram user to Kuberan user
        user_data = api_client.resolve_user(telegram_user_id)
        if not user_data:
            await update.message.reply_text(
                "⚠️ Your Telegram account is not linked.\n"
                "Please link your account in the Kuberan web app first.\n\n"
                "Use /start to get started."
            )
            return

        # Record activity
        api_client.record_activity(telegram_user_id)

        try:
            # Create user-scoped client
            user_client = UserAPIClient(api_client.base_url, user_data['auth_token'])
            accounts = user_client.get_accounts()

            if not accounts:
                await update.message.reply_text(
                    "You don't have any accounts yet.\n"
                    "Create accounts in the web app to get started!"
                )
                return

            message = "💰 *Your Accounts*\n\n"
            total = 0

            for acc in accounts:
                if not acc.get('is_active', True):
                    continue

                balance = acc.get('balance', 0)
                currency = acc.get('currency', 'USD')
                account_type = acc.get('type', 'cash')

                balance_str = format_currency(balance, currency)
                type_emoji = format_account_type(account_type)

                message += f"{type_emoji}\n"
                message += f"*{acc['name']}*\n"
                message += f"Balance: {balance_str}\n\n"

                total += balance

            message += f"━━━━━━━━━━━━━━━━\n"
            message += f"*Total:* {format_currency(total, 'USD')}"

            await update.message.reply_text(message, parse_mode='Markdown')

        except Exception as e:
            logger.error(f"Failed to fetch accounts: {e}")
            await update.message.reply_text(
                "❌ Failed to fetch accounts. Please try again later."
            )

    return balance_command
