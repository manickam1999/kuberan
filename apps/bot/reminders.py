"""Due-date reminders (plan 023, 5.6).

The bot has no HTTP server and the API never calls it, so reminders are
pulled: a repeating job asks the API for the reminders that are due, sends each
linked user a message, and confirms every delivery so it is not sent again.
"""
import logging
from datetime import date, datetime
from typing import Any, Dict, Optional

import httpx
from telegram.constants import ParseMode
from telegram.error import Forbidden, TelegramError
from telegram.ext import ContextTypes

from api_client import KuberanAPIClient
from handlers.common import describe_error
from utils.formatting import esc, format_currency

logger = logging.getLogger(__name__)


def format_due_date(due_on: str, today: Optional[date] = None) -> str:
    """Format a YYYY-MM-DD date as "1 Dec" (with the year when it is not this year)."""
    parsed = datetime.strptime(due_on, "%Y-%m-%d").date()
    today = today or date.today()
    text = f"{parsed.day} {parsed.strftime('%b')}"
    return text if parsed.year == today.year else f"{text} {parsed.year}"


def format_reminder_message(reminder: Dict[str, Any], today: Optional[date] = None) -> str:
    """Build the (HTML) message for one due reminder.

    Amounts are in the account's own currency, as the API sends them.
    """
    name = esc(reminder.get("account_name") or "Your account")
    when = esc(format_due_date(reminder["due_on"], today))
    currency = reminder["currency"]
    amount = format_currency(reminder.get("amount") or 0, currency)
    kind = reminder.get("kind")

    if kind == "fd_maturity":
        return f"🏦 <b>{name}</b> matures on {when} — {esc(amount)} at maturity"
    if kind == "loan_due":
        payment = reminder.get("monthly_payment")
        if payment:
            return (f"🏦 <b>{name}</b> payment of {esc(format_currency(payment, currency))} "
                    f"is due on {when} — {esc(amount)} owed")
        return f"🏦 <b>{name}</b> is due on {when} — {esc(amount)} owed"
    return f"💳 <b>{name}</b> is due on {when} — {esc(amount)} owed"


async def send_due_reminders(context: ContextTypes.DEFAULT_TYPE) -> None:
    """Job: send every due reminder and confirm it with the API.

    ``context.job.data`` is the :class:`KuberanAPIClient`. A reminder is
    confirmed only after Telegram accepted the message, so a transient failure
    is retried on the next poll. A user who blocked the bot (Forbidden) can
    never be reached, so that reminder is confirmed too instead of being
    retried forever.
    """
    api: KuberanAPIClient = context.job.data
    try:
        reminders = await api.get_due_reminders()
    except httpx.HTTPError as e:
        logger.warning("Failed to fetch due reminders: %s", describe_error(e))
        return

    for reminder in reminders:
        reminder_id = reminder.get("id")
        chat_id = reminder.get("telegram_user_id")
        if not reminder_id or not chat_id:
            continue
        try:
            await context.bot.send_message(
                chat_id=chat_id,
                text=format_reminder_message(reminder),
                parse_mode=ParseMode.HTML,
            )
        except Forbidden:
            logger.warning("Reminder %s: user blocked the bot; dropping it", reminder_id)
        except TelegramError as e:
            logger.warning("Reminder %s not sent, will retry: %s", reminder_id, describe_error(e))
            continue
        try:
            await api.mark_reminder_sent(reminder_id)
        except httpx.HTTPError as e:
            # Sent but not confirmed: the next poll may deliver it again.
            logger.warning("Reminder %s sent but not confirmed: %s", reminder_id, describe_error(e))
