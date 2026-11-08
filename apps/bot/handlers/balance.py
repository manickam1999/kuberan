from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, get_base_currency, resolve_linked_user
from utils.formatting import esc, format_currency, format_account_type
import logging

logger = logging.getLogger(__name__)

NOT_LINKED = (
    "⚠️ Your Telegram account is not linked.\n"
    "Please link your account in the Kuberan web app first.\n\n"
    "Use /start to get started."
)


def format_balance_message(accounts: list, base_currency: str) -> str:
    """Build the /balance message (HTML) from the user's accounts.

    Each account is shown in its own currency. The total sums ``balance_base``
    (already converted to the base currency by the API) and never raw balances.
    Accounts without ``balance_base`` (no exchange rate) are left out of the
    total and listed underneath.
    """
    message = "💰 <b>Your Accounts</b>\n\n"
    total = 0
    not_in_total = []

    for acc in accounts:
        if not acc.get('is_active', True):
            continue

        balance = acc.get('balance', 0)
        acc_currency = acc.get('currency') or base_currency
        account_type = acc.get('type', 'cash')

        message += f"{format_account_type(account_type)}\n"
        message += f"<b>{esc(acc.get('name', 'Unnamed Account'))}</b>\n"
        message += f"Balance: {esc(format_currency(balance, acc_currency))}\n\n"

        balance_base = acc.get('balance_base')
        if balance_base is None:
            not_in_total.append((acc, acc_currency))
        else:
            total += balance_base

    message += "━━━━━━━━━━━━━━━━\n"
    message += f"<b>Total ({esc(base_currency)}):</b> {esc(format_currency(total, base_currency))}"

    if not_in_total:
        message += "\n\n<b>Not included in total (no exchange rate)</b>\n"
        for acc, acc_currency in not_in_total:
            name = esc(acc.get('name', 'Unnamed Account'))
            amount = esc(format_currency(acc.get('balance', 0), acc_currency))
            message += f"• {name}: {amount}\n"
        message = message.rstrip("\n")
    return message


def handle(api_client: KuberanAPIClient):
    async def balance_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update, NOT_LINKED)
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

            base_currency = await get_base_currency(user_client)
            await update.message.reply_text(
                format_balance_message(accounts, base_currency), parse_mode=ParseMode.HTML
            )

        except Exception as e:
            logger.error("Failed to fetch accounts: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch accounts. Please try again later."
            )

    return balance_command
