"""Shared helpers for bot handlers."""
import logging
from typing import Any, Dict, Optional

import httpx
from telegram import Update

from api_client import KuberanAPIClient

logger = logging.getLogger(__name__)

NOT_LINKED_MESSAGE = (
    "⚠️ Your Telegram account is not linked.\n"
    "Use /start to link your account."
)
SERVICE_UNAVAILABLE_MESSAGE = (
    "❌ Kuberan is unreachable right now. Please try again later."
)


def describe_error(e: BaseException) -> str:
    """Summarise an exception for logs without URLs, ids or response bodies."""
    if isinstance(e, httpx.HTTPStatusError):
        return f"HTTP {e.response.status_code}"
    return type(e).__name__


async def resolve_linked_user(api_client: KuberanAPIClient, update: Update,
                              not_linked_message: str = NOT_LINKED_MESSAGE
                              ) -> Optional[Dict[str, Any]]:
    """Resolve the sender to a Kuberan user, replying if that is not possible.

    Returns the resolved user (and records activity), or None after telling
    the user their account is not linked or the API is unavailable.
    """
    telegram_user_id = update.effective_user.id
    try:
        user_data = await api_client.resolve_user(telegram_user_id)
    except httpx.HTTPError as e:
        logger.error("Failed to resolve Telegram user: %s", describe_error(e))
        logger.debug("Resolve failure for telegram_user_id=%s", telegram_user_id)
        await update.effective_message.reply_text(SERVICE_UNAVAILABLE_MESSAGE)
        return None

    if not user_data:
        await update.effective_message.reply_text(not_linked_message)
        return None

    await api_client.record_activity(telegram_user_id)
    return user_data
