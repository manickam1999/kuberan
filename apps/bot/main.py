import logging
from telegram import Update
from telegram.ext import Application, CommandHandler
from config import config
from api_client import KuberanAPIClient
from handlers import balance, budgets, accounts, categories, summary, help_cmd
from handlers.start import create_start_conversation
from handlers.transaction_flow import create_transaction_conversation

logging.basicConfig(
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    level=getattr(logging, config.LOG_LEVEL)
)
logger = logging.getLogger(__name__)

def main():
    """Start the Kuberan Telegram bot"""

    # Validate configuration
    if not config.TELEGRAM_BOT_TOKEN:
        logger.error("TELEGRAM_BOT_TOKEN is not set. Please configure the bot token.")
        return

    if not config.BOT_INTERNAL_SECRET:
        logger.error("BOT_INTERNAL_SECRET is not set. Please configure the internal secret.")
        return

    # Create API client
    api_client = KuberanAPIClient(config.API_BASE_URL, config.BOT_INTERNAL_SECRET)

    # Create bot application
    app = Application.builder().token(config.TELEGRAM_BOT_TOKEN).build()

    # Register command handlers
    app.add_handler(create_start_conversation(api_client))
    app.add_handler(CommandHandler("help", help_cmd.handle()))
    app.add_handler(CommandHandler("balance", balance.handle(api_client)))
    app.add_handler(create_transaction_conversation(api_client, "expense"))
    app.add_handler(create_transaction_conversation(api_client, "income"))
    app.add_handler(CommandHandler("budgets", budgets.handle(api_client)))
    app.add_handler(CommandHandler("accounts", accounts.handle(api_client)))
    app.add_handler(CommandHandler("categories", categories.handle(api_client)))
    app.add_handler(CommandHandler("summary", summary.handle(api_client)))

    # Start bot (run_polling manages its own event loop)
    logger.info("Starting Kuberan Telegram bot...")
    logger.info(f"API Base URL: {config.API_BASE_URL}")

    try:
        app.run_polling(allowed_updates=Update.ALL_TYPES)
    except Exception as e:
        logger.error(f"Bot crashed: {e}")
        raise

if __name__ == '__main__':
    main()  # Call directly, no asyncio.run()
