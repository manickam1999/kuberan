"""Receipt photos: attach an image or PDF to a transaction (no OCR).

A file sent within 10 minutes of an entry the bot created is attached to that
entry. Otherwise the bot asks which recent transaction it belongs to.
"""
import logging
from typing import Any, Dict, List, Optional

import httpx
from telegram import InlineKeyboardButton, InlineKeyboardMarkup, Update
from telegram.constants import ParseMode
from telegram.ext import CallbackQueryHandler, ContextTypes, MessageHandler, filters

from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
from handlers.recent import RECENT_PAGE_SIZE, describe_transaction, recent_bot_transactions
from utils.formatting import esc

logger = logging.getLogger(__name__)

PENDING_KEY = "receipt_pending"
# The API accepts JPEG, PNG, WebP and PDF up to MAX_UPLOAD_BYTES (10 MiB).
ALLOWED_TYPES = {
    "image/jpeg": "jpg",
    "image/png": "png",
    "image/webp": "webp",
    "application/pdf": "pdf",
}
MAX_FILE_BYTES = 10 * 1024 * 1024
PICKER_SIZE = 5

RECEIPT_FILTER = filters.ChatType.PRIVATE & (
    filters.PHOTO | filters.Document.IMAGE | filters.Document.PDF
)

UNSUPPORTED_MESSAGE = "I can attach JPEG, PNG or WebP images and PDFs. That file type isn't supported."
TOO_LARGE_MESSAGE = "That file is too large: receipts can be up to 10 MB."


def extract_receipt(message) -> Optional[Dict[str, Any]]:
    """Describe the file in a message: {file_id, filename, content_type, size}.

    Returns None when the message carries no photo or document. Telegram photos
    are always JPEG; documents keep their own name and MIME type.
    """
    if getattr(message, "photo", None):
        photo = message.photo[-1]  # the largest size
        return {
            "file_id": photo.file_id,
            "filename": "receipt.jpg",
            "content_type": "image/jpeg",
            "size": getattr(photo, "file_size", None) or 0,
        }
    document = getattr(message, "document", None)
    if document:
        content_type = (document.mime_type or "").lower()
        ext = ALLOWED_TYPES.get(content_type, "")
        return {
            "file_id": document.file_id,
            "filename": document.file_name or (f"receipt.{ext}" if ext else "receipt"),
            "content_type": content_type,
            "size": getattr(document, "file_size", None) or 0,
        }
    return None


def attach_error_message(error: httpx.HTTPError) -> str:
    """User-facing reason for a failed attach."""
    status = error.response.status_code if isinstance(error, httpx.HTTPStatusError) else None
    if status == 413:
        return TOO_LARGE_MESSAGE
    if status == 415:
        return UNSUPPORTED_MESSAGE
    if status == 409:
        return "That transaction already has the maximum number of receipts."
    if status == 404:
        return "That transaction no longer exists."
    return "❌ Couldn't attach the receipt. Please try again later."


async def attach_receipt(bot, user_client, transaction_id: str, receipt: Dict[str, Any]) -> None:
    """Download the Telegram file and upload it to the transaction."""
    tg_file = await bot.get_file(receipt["file_id"])
    content = bytes(await tg_file.download_as_bytearray())
    await user_client.upload_attachment(
        transaction_id, content, receipt["filename"], receipt["content_type"]
    )


def _picker_keyboard(transactions: List[Dict[str, Any]]) -> InlineKeyboardMarkup:
    rows = [
        [InlineKeyboardButton(describe_transaction(t)[:60], callback_data=f"rc:{t['id']}")]
        for t in transactions
    ]
    rows.append([InlineKeyboardButton("Cancel", callback_data="rc:x")])
    return InlineKeyboardMarkup(rows)


def handle(api_client: KuberanAPIClient) -> MessageHandler:
    async def on_receipt(update: Update, context: ContextTypes.DEFAULT_TYPE):
        message = update.message
        receipt = extract_receipt(message)
        if receipt is None:
            return
        if receipt["content_type"] not in ALLOWED_TYPES:
            await message.reply_text(UNSUPPORTED_MESSAGE)
            return
        if receipt["size"] > MAX_FILE_BYTES:
            await message.reply_text(TOO_LARGE_MESSAGE)
            return

        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return
        user_client = api_client.for_user(user_data["auth_token"])

        try:
            transactions = await user_client.get_recent_transactions(RECENT_PAGE_SIZE)
            recent = recent_bot_transactions(transactions)
            if recent:
                target = recent[0]
                await attach_receipt(context.bot, user_client, str(target["id"]), receipt)
                await message.reply_text(
                    f"📎 Receipt attached to {esc(describe_transaction(target))}",
                    parse_mode=ParseMode.HTML,
                )
                return

            candidates = [t for t in transactions if t.get("type") in ("expense", "income")][:PICKER_SIZE]
            if not candidates:
                await message.reply_text("You don't have any transactions to attach a receipt to yet.")
                return
            context.user_data[PENDING_KEY] = receipt
            await message.reply_text(
                "Which transaction is this receipt for?",
                reply_markup=_picker_keyboard(candidates),
            )
        except httpx.HTTPError as e:
            logger.error("Failed to attach receipt: %s", describe_error(e))
            await message.reply_text(attach_error_message(e))

    return MessageHandler(RECEIPT_FILTER, on_receipt)


def callbacks(api_client: KuberanAPIClient) -> CallbackQueryHandler:
    """Handle the 'which transaction?' buttons."""

    async def on_pick(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()
        transaction_id = (query.data or "").split(":", 1)[-1]

        receipt = context.user_data.pop(PENDING_KEY, None)
        if transaction_id == "x":
            await query.edit_message_text("Cancelled. The receipt wasn't saved.")
            return
        if not receipt:
            await query.edit_message_text("That receipt is no longer waiting. Send it again.")
            return

        try:
            user_data = await api_client.resolve_user(query.from_user.id)
        except httpx.HTTPError as e:
            logger.error("Failed to resolve Telegram user: %s", describe_error(e))
            await query.edit_message_text("❌ Kuberan is unreachable right now. Please send the receipt again later.")
            return
        if not user_data:
            await query.edit_message_text("⚠️ Your Telegram account is not linked. Use /start to link it.")
            return

        user_client = api_client.for_user(user_data["auth_token"])
        try:
            await attach_receipt(context.bot, user_client, transaction_id, receipt)
        except httpx.HTTPError as e:
            logger.error("Failed to attach receipt: %s", describe_error(e))
            await query.edit_message_text(attach_error_message(e))
            return
        await query.edit_message_text("📎 Receipt attached.")

    return CallbackQueryHandler(on_pick, pattern=r"^rc:")
