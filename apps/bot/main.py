import logging
import os
import time

from telegram import Update
from telegram.constants import ChatType
from telegram.ext import (
    Application, ApplicationHandlerStop, CommandHandler, ContextTypes,
    TypeHandler, filters,
)

from config import config
from api_client import KuberanAPIClient
from handlers import balance, budgets, accounts, categories, summary, help_cmd, owed
from handlers.common import describe_error
from handlers.start import create_start_conversation
from handlers.transaction_flow import create_transaction_conversation

logging.basicConfig(
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO)
)
# httpx logs every request URL at INFO, and Telegram Bot API URLs embed the
# bot token (https://api.telegram.org/bot<token>/getUpdates). Keep these
# loggers at WARNING so the token never reaches the logs.
for noisy in ("httpx", "httpcore"):
    logging.getLogger(noisy).setLevel(logging.WARNING)

logger = logging.getLogger(__name__)

# Touched periodically by the job queue; the container healthcheck fails when
# it goes stale (event loop wedged or polling dead).
HEARTBEAT_FILE = os.getenv("BOT_HEARTBEAT_FILE", "/tmp/kuberan-bot-heartbeat")
HEARTBEAT_INTERVAL_SECONDS = 30

ERROR_REPLY = "❌ Something went wrong. Please try again later."


async def reject_non_private(update: object, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Drop every update that does not come from a private chat.

    The bot acts on a single user's finances; replying in groups or channels
    would leak balances to other members. Runs in group -1, before any
    handler, and also covers callback-query updates.
    """
    chat = update.effective_chat if isinstance(update, Update) else None
    if chat is None or chat.type != ChatType.PRIVATE:
        raise ApplicationHandlerStop


async def on_error(update: object, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Log unhandled handler errors and tell the user something went wrong."""
    err = context.error
    logger.error(
        "Unhandled error in handler: %s", describe_error(err) if err else "unknown",
        exc_info=(type(err), err, err.__traceback__) if err else None,
    )
    if isinstance(update, Update) and update.effective_message:
        try:
            await update.effective_message.reply_text(ERROR_REPLY)
        except Exception as reply_err:  # noqa: BLE001 - best effort only
            logger.warning("Failed to send error reply: %s", describe_error(reply_err))


async def heartbeat(context: ContextTypes.DEFAULT_TYPE) -> None:
    """Touch the heartbeat file read by the container healthcheck."""
    try:
        with open(HEARTBEAT_FILE, "w", encoding="utf-8") as f:
            f.write(str(int(time.time())))
    except OSError as e:
        logger.warning("Failed to write heartbeat: %s", describe_error(e))


def build_application(api_client: KuberanAPIClient) -> Application:
    """Create the bot application with all handlers registered."""

    async def post_shutdown(_: Application) -> None:
        await api_client.aclose()

    app = (
        Application.builder()
        .token(config.TELEGRAM_BOT_TOKEN)
        .post_shutdown(post_shutdown)
        .build()
    )

    private = filters.ChatType.PRIVATE

    app.add_handler(TypeHandler(Update, reject_non_private), group=-1)

    app.add_handler(create_start_conversation(api_client))
    app.add_handler(CommandHandler("help", help_cmd.handle(), filters=private))
    app.add_handler(CommandHandler("balance", balance.handle(api_client), filters=private))
    app.add_handler(create_transaction_conversation(api_client, "expense"))
    app.add_handler(create_transaction_conversation(api_client, "income"))
    app.add_handler(CommandHandler("budgets", budgets.handle(api_client), filters=private))
    app.add_handler(CommandHandler("accounts", accounts.handle(api_client), filters=private))
    app.add_handler(CommandHandler("categories", categories.handle(api_client), filters=private))
    app.add_handler(CommandHandler("summary", summary.handle(api_client), filters=private))
    app.add_handler(CommandHandler("owed", owed.handle(api_client), filters=private))

    app.add_error_handler(on_error)

    if app.job_queue is None:
        # Without the job-queue extra, conversation timeouts silently never fire.
        raise RuntimeError("JobQueue unavailable: install python-telegram-bot[job-queue]")
    app.job_queue.run_repeating(heartbeat, interval=HEARTBEAT_INTERVAL_SECONDS, first=0)

    return app


def main():
    """Start the Kuberan Telegram bot"""

    # Validate configuration
    if not config.TELEGRAM_BOT_TOKEN:
        logger.error("TELEGRAM_BOT_TOKEN is not set. Please configure the bot token.")
        raise SystemExit(1)

    if not config.BOT_INTERNAL_SECRET:
        logger.error("BOT_INTERNAL_SECRET is not set. Please configure the internal secret.")
        raise SystemExit(1)

    api_client = KuberanAPIClient(config.API_BASE_URL, config.BOT_INTERNAL_SECRET)
    app = build_application(api_client)

    logger.info("Starting Kuberan Telegram bot (API base URL: %s)", config.API_BASE_URL)

    # run_polling manages its own event loop. Only the update types the
    # handlers use are requested.
    app.run_polling(allowed_updates=[Update.MESSAGE, Update.CALLBACK_QUERY])


if __name__ == '__main__':
    main()
