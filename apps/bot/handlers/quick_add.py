"""One-line quick add: type "coffee 18.40" and the expense is recorded.

The text entry point is registered inside the expense ``ConversationHandler``
(see ``create_transaction_conversation(extra_entry_points=...)``), never as a
global handler: a global one would catch the text a conversation is waiting
for and record a second transaction. The inline buttons on the reply are
stateless (they carry the transaction id and re-read everything from the
API), so they keep working after the conversation has ended.
"""
import logging
from collections import OrderedDict
from typing import Any, Dict, List, Optional

import httpx
from telegram import InlineKeyboardButton, InlineKeyboardMarkup, Update
from telegram.constants import ParseMode
from telegram.ext import (
    CallbackQueryHandler, ContextTypes, ConversationHandler, MessageHandler,
)

from api_client import KuberanAPIClient
from handlers.common import NOT_LINKED_MESSAGE, SERVICE_UNAVAILABLE_MESSAGE, describe_error
from handlers.transaction_flow import (
    ACCOUNT, PRIVATE_TEXT, SPENDING_ACCOUNT_TYPES, TXN_KEY,
    _build_account_keyboard, _extract_account_and_category,
    _order_categories_hierarchically, _resolve_user_and_setup,
)
from utils.formatting import display_icon, esc, format_currency
from utils.parsing import currency_matches_hint, describe_hint, parse_quick_entry

logger = logging.getLogger(__name__)

# The API caps a description at 500 characters.
MAX_DESCRIPTION_LEN = 500
# How many Telegram update ids are remembered to drop a replayed update.
SEEN_UPDATES_LIMIT = 200
SEEN_KEY = "quick_add_seen"

CATEGORY_PAGE_SIZE = 9

USAGE_HINT = (
    "I couldn't find an amount. Send it with what it was for, like "
    "<code>coffee 18.40</code> or <code>18.40 coffee</code>, "
    "or use /expense or /income for the guided flow."
)
NEEDS_DESCRIPTION_HINT = (
    "What was it for? Send the amount with a description, like "
    "<code>coffee 18.40</code>."
)
IN_PROGRESS_HINT = (
    "You're in the middle of another entry. Finish it with the buttons above, "
    "or send /cancel to start over."
)


def already_handled(context: ContextTypes.DEFAULT_TYPE, update_id: int) -> bool:
    """Record a Telegram update id; True if this process already saw it."""
    seen: "OrderedDict[int, bool]" = context.application.bot_data.setdefault(SEEN_KEY, OrderedDict())
    if update_id in seen:
        return True
    seen[update_id] = True
    while len(seen) > SEEN_UPDATES_LIMIT:
        seen.popitem(last=False)
    return False


def _account_lookup(accounts: List[Dict[str, Any]], account_id: Optional[str]) -> Dict[str, Any]:
    return next((a for a in accounts if str(a.get("id")) == str(account_id)), {})


def _category_lookup(categories: List[Dict[str, Any]], category_id: Optional[str]) -> Dict[str, Any]:
    if not category_id:
        return {}
    return next((c for c in categories if str(c.get("id")) == str(category_id)), {})


def format_added_line(amount: int, currency: str, category: Dict[str, Any],
                      account_name: str, verb: str = "Added") -> str:
    """'Added RM 18.40 · Dining · DBS Everyday' as HTML."""
    cat_name = category.get("name")
    if cat_name:
        icon = display_icon(category.get("icon", ""))
        cat_text = f"{icon} {cat_name}".strip() if icon else cat_name
    else:
        cat_text = "Uncategorized"
    return (
        f"<b>{verb} {esc(format_currency(amount, currency))}</b>"
        f" · {esc(cat_text)} · {esc(account_name or 'Unknown account')}"
    )


def build_entry_keyboard(transaction_id: str) -> InlineKeyboardMarkup:
    """Change category / change account / undo, all keyed by transaction id."""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("Change category", callback_data=f"qa:c:{transaction_id}"),
            InlineKeyboardButton("Change account", callback_data=f"qa:a:{transaction_id}"),
        ],
        [InlineKeyboardButton("Undo", callback_data=f"qa:u:{transaction_id}")],
    ])


def _currency_note(entry_hint, account_currency: str) -> str:
    if currency_matches_hint(entry_hint, account_currency):
        return ""
    return (
        f"\n<i>You typed {esc(describe_hint(entry_hint))}, but this account is in "
        f"{esc(account_currency)}: the amount was recorded as {esc(account_currency)}, "
        f"not converted.</i>"
    )


async def _suggest_category(user_client, description: str, amount: int,
                            account_id: str) -> Optional[Dict[str, Any]]:
    """Best-effort category suggestion; a failure just means no suggestion."""
    try:
        return await user_client.suggest_category(description, "expense", amount, account_id)
    except httpx.HTTPError as e:
        logger.warning("Category suggestion failed: %s", describe_error(e))
        return None


def quick_add_entry_point(api_client: KuberanAPIClient) -> MessageHandler:
    """Build the plain-text entry point to add to the expense conversation."""

    async def quick_add(update: Update, context: ContextTypes.DEFAULT_TYPE):
        message = update.message
        if already_handled(context, update.update_id):
            return ConversationHandler.END

        # Another entry is waiting for a button tap (the income conversation, or
        # one whose state this handler cannot see): never start a second one.
        if context.user_data.get(TXN_KEY):
            await message.reply_text(IN_PROGRESS_HINT)
            return ConversationHandler.END

        entry = parse_quick_entry(message.text or "")
        if entry is None:
            await message.reply_text(USAGE_HINT, parse_mode=ParseMode.HTML)
            return ConversationHandler.END
        if not entry.description:
            await message.reply_text(NEEDS_DESCRIPTION_HINT, parse_mode=ParseMode.HTML)
            return ConversationHandler.END
        if entry.amount <= 0:
            await message.reply_text("The amount must be greater than zero.")
            return ConversationHandler.END
        if len(entry.description) > MAX_DESCRIPTION_LEN:
            await message.reply_text(f"That description is too long (max {MAX_DESCRIPTION_LEN} characters).")
            return ConversationHandler.END

        user_client, error = await _resolve_user_and_setup(api_client, message.from_user.id, context)
        if error:
            await message.reply_text(error)
            return ConversationHandler.END

        txn = context.user_data[TXN_KEY]
        txn["type"] = "expense"
        description, matched_acc, matched_cat = _extract_account_and_category(
            entry.description, txn["accounts"], txn["categories"]
        )
        description = description or entry.description
        if matched_acc:
            txn["account_id"] = matched_acc["id"]
            txn["account_name"] = matched_acc["name"]
            if matched_acc.get("currency"):
                txn["currency"] = matched_acc["currency"]
        txn["amount"] = entry.amount
        txn["description"] = description

        suggestion = None
        if matched_cat:
            txn["category_id"] = matched_cat["id"]
            txn["category_name"] = matched_cat["name"]
            txn["category_icon"] = matched_cat.get("icon", "")
        else:
            suggestion = await _suggest_category(user_client, description, entry.amount, txn["account_id"])
            # A "history" suggestion is the only one the API does not apply on
            # its own; a "rule" match is applied by POST /transactions itself.
            if suggestion and suggestion.get("reason") == "history":
                txn["category_id"] = suggestion["category_id"]
                txn["category_name"] = suggestion.get("category_name")

        if not matched_acc and not txn.get("default_confident"):
            # No saved default account: ask which account, then the guided flow's
            # confirm card takes over.
            if suggestion and not txn.get("category_id"):
                txn["category_id"] = suggestion["category_id"]
                txn["category_name"] = suggestion.get("category_name")
            await message.reply_text(
                "Which account was it from?",
                reply_markup=_build_account_keyboard(txn["accounts"]),
            )
            return ACCOUNT

        payload = {
            "type": "expense",
            "account_id": txn["account_id"],
            "amount": entry.amount,
            "description": description,
        }
        if txn.get("category_id"):
            payload["category_id"] = txn["category_id"]

        try:
            result = await user_client.create_transaction(payload)
        except httpx.HTTPError as e:
            logger.error("Quick add failed to create transaction: %s", describe_error(e))
            context.user_data.pop(TXN_KEY, None)
            await message.reply_text("Failed to record that. Please try again.")
            return ConversationHandler.END

        created = result.get("transaction", {}) if isinstance(result, dict) else {}
        account = _account_lookup(txn["accounts"], created.get("account_id") or txn["account_id"])
        currency = account.get("currency") or txn["currency"]
        category = _category_lookup(txn["categories"], created.get("category_id") or txn.get("category_id"))
        if not category and suggestion and created.get("category_id") == suggestion.get("category_id"):
            category = {"name": suggestion.get("category_name")}

        text = format_added_line(entry.amount, currency, category,
                                 account.get("name") or txn["account_name"])
        text += _currency_note(entry.currency_hint, currency)
        context.user_data.pop(TXN_KEY, None)
        await message.reply_text(
            text,
            reply_markup=build_entry_keyboard(str(created.get("id"))) if created.get("id") else None,
            parse_mode=ParseMode.HTML,
        )
        return ConversationHandler.END

    return MessageHandler(PRIVATE_TEXT, quick_add)


# --- inline buttons under the "Added ..." reply -------------------------------

async def _fetch_context(user_client):
    return await user_client.get_accounts(), await user_client.get_categories()


def _expense_categories(categories: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    return _order_categories_hierarchically([c for c in categories if c.get("type") == "expense"])


def _category_picker(transaction_id: str, categories: List[Dict[str, Any]], page: int) -> InlineKeyboardMarkup:
    ordered = _expense_categories(categories)
    start = page * CATEGORY_PAGE_SIZE
    chunk = ordered[start:start + CATEGORY_PAGE_SIZE]
    rows = []
    for i in range(0, len(chunk), 3):
        rows.append([
            InlineKeyboardButton(
                f"{display_icon(c.get('icon', ''))} {c['name']}".strip(),
                callback_data=f"qa:sc:{transaction_id}:{start + i + j}",
            )
            for j, c in enumerate(chunk[i:i + 3])
        ])
    nav = []
    if page > 0:
        nav.append(InlineKeyboardButton("< Prev", callback_data=f"qa:cp:{transaction_id}:{page - 1}"))
    if start + CATEGORY_PAGE_SIZE < len(ordered):
        nav.append(InlineKeyboardButton("Next >", callback_data=f"qa:cp:{transaction_id}:{page + 1}"))
    if nav:
        rows.append(nav)
    rows.append([InlineKeyboardButton("Back", callback_data=f"qa:b:{transaction_id}")])
    return InlineKeyboardMarkup(rows)


def _account_picker(transaction_id: str, accounts: List[Dict[str, Any]]) -> InlineKeyboardMarkup:
    eligible = [a for a in accounts if a.get("type") in SPENDING_ACCOUNT_TYPES and a.get("is_active", True)]
    multi_currency = len({a.get("currency") for a in eligible}) > 1
    rows = []
    for i in range(0, len(eligible), 2):
        rows.append([
            InlineKeyboardButton(
                f"{a['name']} ({a['currency']})" if multi_currency and a.get("currency") else a["name"],
                callback_data=f"qa:sa:{transaction_id}:{i + j}",
            )
            for j, a in enumerate(eligible[i:i + 2])
        ])
    rows.append([InlineKeyboardButton("Back", callback_data=f"qa:b:{transaction_id}")])
    return InlineKeyboardMarkup(rows)


def _eligible_accounts(accounts: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    return [a for a in accounts if a.get("type") in SPENDING_ACCOUNT_TYPES and a.get("is_active", True)]


def quick_add_callbacks(api_client: KuberanAPIClient) -> CallbackQueryHandler:
    """Handle the Change category / Change account / Undo buttons."""

    async def render_entry(query, user_client, transaction_id: str, accounts=None, categories=None,
                           verb: str = "Added"):
        transaction = await user_client.get_transaction(transaction_id)
        if accounts is None or categories is None:
            accounts, categories = await _fetch_context(user_client)
        account = _account_lookup(accounts, transaction.get("account_id"))
        category = _category_lookup(categories, transaction.get("category_id"))
        await query.edit_message_text(
            format_added_line(transaction.get("amount") or 0, account.get("currency") or "MYR",
                              category, account.get("name") or "", verb),
            reply_markup=build_entry_keyboard(transaction_id),
            parse_mode=ParseMode.HTML,
        )

    async def on_button(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        parts = (query.data or "").split(":")
        if len(parts) < 3:
            return
        action, transaction_id = parts[1], parts[2]
        extra = parts[3] if len(parts) > 3 else None

        try:
            user_data = await api_client.resolve_user(query.from_user.id)
        except httpx.HTTPError as e:
            logger.error("Failed to resolve Telegram user: %s", describe_error(e))
            await query.edit_message_text(SERVICE_UNAVAILABLE_MESSAGE)
            return
        if not user_data:
            await query.edit_message_text(NOT_LINKED_MESSAGE)
            return
        user_client = api_client.for_user(user_data["auth_token"])

        try:
            if action == "u":
                await _undo(query, user_client, transaction_id)
            elif action == "b":
                await render_entry(query, user_client, transaction_id)
            elif action in ("c", "cp"):
                categories = await user_client.get_categories()
                page = int(extra) if extra and extra.isdigit() else 0
                await query.edit_message_reply_markup(
                    reply_markup=_category_picker(transaction_id, categories, page))
            elif action == "sc":
                categories = await user_client.get_categories()
                ordered = _expense_categories(categories)
                index = int(extra) if extra and extra.isdigit() else -1
                if not 0 <= index < len(ordered):
                    await query.answer("That category list changed. Try again.", show_alert=True)
                    return
                await user_client.update_transaction(transaction_id, {"category_id": ordered[index]["id"]})
                await render_entry(query, user_client, transaction_id, categories=categories,
                                   accounts=await user_client.get_accounts(), verb="Updated")
            elif action == "a":
                accounts = await user_client.get_accounts()
                await query.edit_message_reply_markup(reply_markup=_account_picker(transaction_id, accounts))
            elif action == "sa":
                accounts = await user_client.get_accounts()
                eligible = _eligible_accounts(accounts)
                index = int(extra) if extra and extra.isdigit() else -1
                if not 0 <= index < len(eligible):
                    await query.answer("That account list changed. Try again.", show_alert=True)
                    return
                await user_client.update_transaction(transaction_id, {"account_id": eligible[index]["id"]})
                await render_entry(query, user_client, transaction_id, accounts=accounts,
                                   categories=await user_client.get_categories(), verb="Updated")
        except httpx.HTTPStatusError as e:
            logger.error("Quick add button failed: %s", describe_error(e))
            if e.response.status_code == 404:
                await query.edit_message_text("That entry no longer exists.")
            else:
                await query.answer("Something went wrong. Please try again.", show_alert=True)
        except httpx.HTTPError as e:
            logger.error("Quick add button failed: %s", describe_error(e))
            await query.answer("Kuberan is unreachable right now.", show_alert=True)

    return CallbackQueryHandler(on_button, pattern=r"^qa:")


async def _undo(query, user_client, transaction_id: str) -> None:
    try:
        await user_client.delete_transaction(transaction_id)
    except httpx.HTTPStatusError as e:
        if e.response.status_code != 404:
            raise
        await query.edit_message_text("That entry was already removed.")
        return
    previous = (getattr(query.message, "text", None) or "").strip()
    await query.edit_message_text(
        "↩️ Undone." + (f"\n<s>{esc(previous)}</s>" if previous else ""),
        parse_mode=ParseMode.HTML,
    )
