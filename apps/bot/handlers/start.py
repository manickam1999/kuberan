from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.constants import ParseMode
from telegram.ext import (
    ContextTypes, ConversationHandler, CommandHandler,
    CallbackQueryHandler, MessageHandler, filters
)
import httpx
from api_client import KuberanAPIClient
from handlers.common import SERVICE_UNAVAILABLE_MESSAGE, describe_error
from utils.formatting import esc
import logging

logger = logging.getLogger(__name__)

PICK_CURRENCY, CUSTOM_CURRENCY = range(2)

LINK_KEY = 'link'


def _build_currency_keyboard():
    """Build inline keyboard for currency selection."""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("🇲🇾 MYR (Default)", callback_data="cur:MYR"),
            InlineKeyboardButton("Other", callback_data="cur:other"),
        ],
    ])


def create_start_conversation(api_client: KuberanAPIClient) -> ConversationHandler:
    """Factory that returns a ConversationHandler for the /start linking flow."""

    async def start_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        telegram_user_id = update.message.from_user.id

        # Check if already linked
        try:
            user_data = await api_client.resolve_user(telegram_user_id)
        except httpx.HTTPError as e:
            logger.error("Failed to resolve Telegram user: %s", describe_error(e))
            await update.message.reply_text(SERVICE_UNAVAILABLE_MESSAGE)
            return ConversationHandler.END

        if user_data:
            await update.message.reply_text(
                "You're already linked to Kuberan!\n\n"
                "Use /help to see available commands."
            )
            return ConversationHandler.END

        # Check if there's a link code
        if not context.args:
            await update.message.reply_text(
                "Welcome to Kuberan!\n\n"
                "To link your account:\n"
                "1. Go to Settings in the Kuberan web app\n"
                "2. Click 'Link Telegram'\n"
                "3. Copy your link code\n"
                "4. Send: /start <code>\n\n"
                "Use /help to see what I can do."
            )
            return ConversationHandler.END

        # Store link info for after currency selection
        context.user_data[LINK_KEY] = {
            'link_code': context.args[0],
            'telegram_user_id': telegram_user_id,
            'username': update.message.from_user.username or "",
            'first_name': update.message.from_user.first_name or "",
        }

        await update.message.reply_text(
            "Choose your default currency:",
            reply_markup=_build_currency_keyboard()
        )
        return PICK_CURRENCY

    async def handle_currency_pick(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        link_data = context.user_data.get(LINK_KEY)
        if not link_data:
            await query.edit_message_text("Session expired. Please try /start again.")
            return ConversationHandler.END

        data = query.data
        if data == "cur:other":
            await query.edit_message_text(
                "Type your currency code (3 letters, e.g. JPY, CAD, AUD):"
            )
            return CUSTOM_CURRENCY

        currency = data.split(":")[1]
        return await _complete_linking(query.edit_message_text, api_client, link_data, currency, context)

    async def handle_custom_currency(update: Update, context: ContextTypes.DEFAULT_TYPE):
        link_data = context.user_data.get(LINK_KEY)
        if not link_data:
            await update.message.reply_text("Session expired. Please try /start again.")
            return ConversationHandler.END

        currency = update.message.text.strip().upper()
        if len(currency) != 3 or not currency.isalpha():
            await update.message.reply_text(
                "Please enter a valid 3-letter currency code (e.g. JPY, CAD, AUD):"
            )
            return CUSTOM_CURRENCY

        return await _complete_linking(update.message.reply_text, api_client, link_data, currency, context)

    async def _complete_linking(reply_func, api_client, link_data, currency, context):
        """Complete the linking process with the chosen currency."""
        try:
            await api_client.complete_link(
                link_data['link_code'],
                link_data['telegram_user_id'],
                link_data['username'],
                link_data['first_name'],
                default_currency=currency,
            )
            await reply_func(
                f"Your Telegram account is now linked to Kuberan!\n"
                f"Default currency: <b>{esc(currency)}</b>\n\n"
                f"Use /help to see available commands.",
                parse_mode=ParseMode.HTML
            )
            logger.info("Linked a Telegram account (currency %s)", currency)
            logger.debug("Linked telegram_user_id=%s", link_data['telegram_user_id'])
        except httpx.HTTPError as e:
            # Never log the link code: it is a short-lived credential.
            logger.error("Failed to complete Telegram link: %s", describe_error(e))
            await reply_func(
                "Invalid or expired link code.\n"
                "Please generate a new code from the web app."
            )

        context.user_data.pop(LINK_KEY, None)
        return ConversationHandler.END

    async def cancel(update: Update, context: ContextTypes.DEFAULT_TYPE):
        context.user_data.pop(LINK_KEY, None)
        await update.message.reply_text("Cancelled.")
        return ConversationHandler.END

    return ConversationHandler(
        entry_points=[CommandHandler("start", start_command, filters=filters.ChatType.PRIVATE)],
        states={
            PICK_CURRENCY: [CallbackQueryHandler(handle_currency_pick, pattern=r'^cur:')],
            CUSTOM_CURRENCY: [MessageHandler(
                filters.ChatType.PRIVATE & filters.TEXT & ~filters.COMMAND, handle_custom_currency
            )],
        },
        fallbacks=[CommandHandler('cancel', cancel, filters=filters.ChatType.PRIVATE)],
        conversation_timeout=120,
        per_message=False,
        per_chat=True,
    )
