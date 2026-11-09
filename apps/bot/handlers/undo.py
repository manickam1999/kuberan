"""/undo: delete the last transaction the bot created in the last 10 minutes."""
import logging

import httpx
from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes

from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
from handlers.recent import RECENT_PAGE_SIZE, describe_transaction, recent_bot_transactions
from utils.formatting import esc

logger = logging.getLogger(__name__)

NOTHING_TO_UNDO = (
    "Nothing to undo: you haven't added a transaction from Telegram in the last 10 minutes."
)


def handle(api_client: KuberanAPIClient):
    async def undo_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data["auth_token"])
            # The list endpoint cannot filter by source, so look at the newest
            # page and pick the bot's entries (source "telegram") client-side.
            recent = recent_bot_transactions(await user_client.get_recent_transactions(RECENT_PAGE_SIZE))
            if not recent:
                await update.message.reply_text(NOTHING_TO_UNDO)
                return
            target = recent[0]
            try:
                await user_client.delete_transaction(str(target["id"]))
            except httpx.HTTPStatusError as e:
                if e.response.status_code != 404:
                    raise
                await update.message.reply_text("That entry was already removed.")
                return
            await update.message.reply_text(
                f"↩️ Removed {esc(describe_transaction(target))}",
                parse_mode=ParseMode.HTML,
            )
        except httpx.HTTPError as e:
            logger.error("Failed to undo: %s", describe_error(e))
            await update.message.reply_text("❌ Couldn't undo that. Please try again later.")

    return undo_command
