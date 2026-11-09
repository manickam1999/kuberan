from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, get_base_currency, resolve_linked_user
from utils.formatting import esc, format_currency, format_percentage
import logging

logger = logging.getLogger(__name__)


def format_budgets_message(budgets: list, progress_by_budget: dict, base_currency: str) -> str:
    """Build the /budgets message (HTML).

    Each budget is shown in its own currency (the progress entry's currency
    when present, else the budget's, else the base currency).
    """
    message = "📊 <b>Your Budgets</b>\n\n"

    for budget in budgets:
        if not budget.get('is_active', True):
            continue

        name = esc(budget.get('name', 'Unnamed Budget'))
        amount = budget.get('amount', 0)
        period = esc(budget.get('period', 'monthly'))

        progress = progress_by_budget.get(budget.get('id'))
        currency = (
            (progress or {}).get('currency') or budget.get('currency') or base_currency
        )
        if progress:
            spent = progress.get('spent', 0)
            remaining = progress.get('remaining', 0)
            percentage = progress.get('percentage', 0)

            status_emoji = "✅" if percentage < 80 else "⚠️" if percentage < 100 else "🚨"

            message += f"{status_emoji} <b>{name}</b> ({period})\n"
            message += f"Budget: {esc(format_currency(amount, currency))}\n"
            message += f"Spent: {esc(format_currency(spent, currency))} ({format_percentage(percentage)})\n"
            message += f"Remaining: {esc(format_currency(remaining, currency))}\n\n"
        else:
            # No matching progress entry; show budget info only.
            message += f"📋 <b>{name}</b> ({period})\n"
            message += f"Budget: {esc(format_currency(amount, currency))}\n\n"

    return message


def handle(api_client: KuberanAPIClient):
    async def budgets_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])
            budgets = await user_client.get_budgets()

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
                for p in await user_client.get_budgets_progress()
                if p.get('budget_id')
            }

            base_currency = await get_base_currency(user_client)
            await update.message.reply_text(
                format_budgets_message(budgets, progress_by_budget, base_currency),
                parse_mode=ParseMode.HTML,
            )

        except Exception as e:
            logger.error("Failed to fetch budgets: %s", describe_error(e))
            await update.message.reply_text(
                "❌ Failed to fetch budgets. Please try again later."
            )

    return budgets_command
