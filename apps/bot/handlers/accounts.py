from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
from utils.formatting import esc, format_currency, format_account_type
import logging

logger = logging.getLogger(__name__)


def format_accounts_message(accounts: list) -> str:
    """Build the /accounts message (HTML)."""
    message = "🏦 <b>Your Accounts</b>\n\n"

    for acc in accounts:
        status = "✅" if acc.get('is_active', True) else "🔒"
        account_type = format_account_type(acc.get('type', 'cash'))
        name = acc.get('name', 'Unnamed Account')
        currency = acc.get('currency', 'USD')
        balance_str = format_currency(acc.get('balance', 0), currency)

        message += f"{status} {account_type}\n"
        message += f"<b>{esc(name)}</b>\n"
        message += f"Balance: {esc(balance_str)}\n"

        # Add extra info for specific account types
        if acc.get('type') == 'credit_card':
            credit_limit = acc.get('credit_limit', 0)
            if credit_limit:
                message += f"Limit: {esc(format_currency(credit_limit, currency))}\n"

        message += "\n"

    return message


def handle(api_client: KuberanAPIClient):
    async def accounts_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])
            accounts = await user_client.get_accounts()

            if not accounts:
                await update.message.reply_text(
                    "You don't have any accounts yet.\n"
                    "Create accounts in the web app to get started!"
                )
                return

            await update.message.reply_text(
                format_accounts_message(accounts), parse_mode=ParseMode.HTML
            )

        except Exception as e:
            logger.error("Failed to fetch accounts: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch accounts. Please try again later."
            )

    return accounts_command
