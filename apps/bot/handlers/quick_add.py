"""One-line quick add: type "coffee 18.40" and confirm the expense.

The text entry point is registered inside the expense ``ConversationHandler``
(see ``create_transaction_conversation(extra_entry_points=...)``), never as a
global handler: a global one would catch the text a conversation is waiting
for and record a second transaction.

Nothing is created by the text itself. The reply is a preview ("Add RM 18.40 ·
Dining · DBS Everyday?") with Add / Change category / Change account / Cancel
buttons. The pending entry lives in ``context.user_data`` under a short random
id carried in the callback data (``qa:<action>:<pendingid>``) and expires after
ten minutes. Tapping Add creates the transaction; the reply then carries an
Undo button keyed by the transaction id, which is stateless (it re-reads
everything from the API), so it keeps working after the conversation has ended.
"""
import logging
import secrets
import time
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

PENDING_KEY = "qa_pending"
# A preview that is not confirmed within this many seconds is dropped.
PENDING_TTL_SECONDS = 600
MAX_PENDING = 20
EXPIRED_MESSAGE = "This entry expired — send it again."

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
                      account_name: str, verb: str = "Added", suffix: str = "") -> str:
    """'Added RM 18.40 · Dining · DBS Everyday' as HTML (suffix: '?' on a preview)."""
    cat_name = category.get("name")
    if cat_name:
        icon = display_icon(category.get("icon", ""))
        cat_text = f"{icon} {cat_name}".strip() if icon else cat_name
    else:
        cat_text = "Uncategorized"
    return (
        f"<b>{verb} {esc(format_currency(amount, currency))}</b>"
        f" · {esc(cat_text)} · {esc(account_name or 'Unknown account')}{suffix}"
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


def _currency_note(entry_hint, account_currency: str, tense: str = "was") -> str:
    if currency_matches_hint(entry_hint, account_currency):
        return ""
    return (
        f"\n<i>You typed {esc(describe_hint(entry_hint))}, but this account is in "
        f"{esc(account_currency)}: the amount {tense} recorded as {esc(account_currency)}, "
        f"not converted.</i>"
    )


def build_undo_keyboard(transaction_id: str) -> InlineKeyboardMarkup:
    """The single Undo button under an 'Added ...' line."""
    return InlineKeyboardMarkup([[InlineKeyboardButton("Undo", callback_data=f"qa:u:{transaction_id}")]])


def build_preview_keyboard(pending_id: str) -> InlineKeyboardMarkup:
    """Add / change category / change account / cancel, keyed by pending id."""
    return InlineKeyboardMarkup([
        [InlineKeyboardButton("Add", callback_data=f"qa:pa:{pending_id}")],
        [
            InlineKeyboardButton("Change category", callback_data=f"qa:pc:{pending_id}"),
            InlineKeyboardButton("Change account", callback_data=f"qa:pk:{pending_id}"),
        ],
        [InlineKeyboardButton("Cancel", callback_data=f"qa:px:{pending_id}")],
    ])


# --- pending entries ------------------------------------------------------------

def _now() -> float:
    return time.time()


def _pending_store(context: ContextTypes.DEFAULT_TYPE) -> Dict[str, Dict[str, Any]]:
    return context.user_data.setdefault(PENDING_KEY, {})


def _is_expired(pending: Dict[str, Any]) -> bool:
    return _now() - pending["created"] > PENDING_TTL_SECONDS


def _store_pending(context: ContextTypes.DEFAULT_TYPE, pending: Dict[str, Any]) -> str:
    """Keep a pending entry under a fresh short id, dropping expired/oldest ones."""
    store = _pending_store(context)
    for key in [k for k, v in store.items() if _is_expired(v)]:
        del store[key]
    while len(store) >= MAX_PENDING:
        del store[min(store, key=lambda k: store[k]["created"])]
    pending["created"] = _now()
    pending_id = secrets.token_urlsafe(6)
    while pending_id in store:
        pending_id = secrets.token_urlsafe(6)
    store[pending_id] = pending
    return pending_id


def _pending_category(pending: Dict[str, Any]) -> Dict[str, Any]:
    category = _category_lookup(pending["categories"], pending.get("category_id") or pending.get("suggested_id"))
    if not category and pending.get("category_name"):
        category = {"name": pending["category_name"]}
    return category


def render_preview(pending: Dict[str, Any]) -> str:
    """'Add RM 18.40 · Dining · DBS Everyday?' plus the not-converted note."""
    text = format_added_line(pending["amount"], pending["currency"], _pending_category(pending),
                             pending["account_name"], verb="Add", suffix="?")
    return text + _currency_note(pending["currency_hint"], pending["currency"], tense="will be")


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

        # Nothing is created yet: keep the entry and ask for a confirmation.
        pending = {
            "account_id": txn["account_id"],
            "account_name": txn["account_name"],
            "currency": txn["currency"],
            "amount": entry.amount,
            "description": description,
            "currency_hint": entry.currency_hint,
            # category_id is what gets sent: a name typed in the text, a "history"
            # suggestion or the user's pick. A "rule" match is only shown
            # (suggested_id); POST /transactions applies it itself.
            "category_id": txn.get("category_id"),
            "suggested_id": suggestion.get("category_id") if suggestion and not matched_cat else None,
            "category_name": txn.get("category_name")
            or (suggestion.get("category_name") if suggestion and not matched_cat else None),
            "explicit_category": bool(matched_cat),
            "accounts": txn["accounts"],
            "categories": txn["categories"],
        }
        context.user_data.pop(TXN_KEY, None)
        pending_id = _store_pending(context, pending)
        await message.reply_text(
            render_preview(pending),
            reply_markup=build_preview_keyboard(pending_id),
            parse_mode=ParseMode.HTML,
        )
        return ConversationHandler.END

    return MessageHandler(PRIVATE_TEXT, quick_add)


# --- inline buttons under the "Added ..." reply -------------------------------

async def _fetch_context(user_client):
    return await user_client.get_accounts(), await user_client.get_categories()


def _expense_categories(categories: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    return _order_categories_hierarchically([c for c in categories if c.get("type") == "expense"])


def _category_picker(transaction_id: str, categories: List[Dict[str, Any]], page: int,
                     pick: str = "sc", nav_action: str = "cp", back: str = "b") -> InlineKeyboardMarkup:
    ordered = _expense_categories(categories)
    start = page * CATEGORY_PAGE_SIZE
    chunk = ordered[start:start + CATEGORY_PAGE_SIZE]
    rows = []
    for i in range(0, len(chunk), 3):
        rows.append([
            InlineKeyboardButton(
                f"{display_icon(c.get('icon', ''))} {c['name']}".strip(),
                callback_data=f"qa:{pick}:{transaction_id}:{start + i + j}",
            )
            for j, c in enumerate(chunk[i:i + 3])
        ])
    nav = []
    if page > 0:
        nav.append(InlineKeyboardButton("< Prev", callback_data=f"qa:{nav_action}:{transaction_id}:{page - 1}"))
    if start + CATEGORY_PAGE_SIZE < len(ordered):
        nav.append(InlineKeyboardButton("Next >", callback_data=f"qa:{nav_action}:{transaction_id}:{page + 1}"))
    if nav:
        rows.append(nav)
    rows.append([InlineKeyboardButton("Back", callback_data=f"qa:{back}:{transaction_id}")])
    return InlineKeyboardMarkup(rows)


def _account_picker(transaction_id: str, accounts: List[Dict[str, Any]],
                    pick: str = "sa", back: str = "b") -> InlineKeyboardMarkup:
    eligible = [a for a in accounts if a.get("type") in SPENDING_ACCOUNT_TYPES and a.get("is_active", True)]
    multi_currency = len({a.get("currency") for a in eligible}) > 1
    rows = []
    for i in range(0, len(eligible), 2):
        rows.append([
            InlineKeyboardButton(
                f"{a['name']} ({a['currency']})" if multi_currency and a.get("currency") else a["name"],
                callback_data=f"qa:{pick}:{transaction_id}:{i + j}",
            )
            for j, a in enumerate(eligible[i:i + 2])
        ])
    rows.append([InlineKeyboardButton("Back", callback_data=f"qa:{back}:{transaction_id}")])
    return InlineKeyboardMarkup(rows)


def _eligible_accounts(accounts: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    return [a for a in accounts if a.get("type") in SPENDING_ACCOUNT_TYPES and a.get("is_active", True)]


PENDING_ACTIONS = ("pa", "px", "pc", "pcp", "psc", "pk", "psk", "pb")


def quick_add_callbacks(api_client: KuberanAPIClient) -> CallbackQueryHandler:
    """Handle the preview buttons (Add / Change / Cancel) and the Undo button."""

    async def user_client_for(query):
        """The caller's API client, or None after telling them why not."""
        try:
            user_data = await api_client.resolve_user(query.from_user.id)
        except httpx.HTTPError as e:
            logger.error("Failed to resolve Telegram user: %s", describe_error(e))
            await query.edit_message_text(SERVICE_UNAVAILABLE_MESSAGE)
            return None
        if not user_data:
            await query.edit_message_text(NOT_LINKED_MESSAGE)
            return None
        return api_client.for_user(user_data["auth_token"])

    async def show_preview(query, pending_id: str, pending: Dict[str, Any]):
        await query.edit_message_text(
            render_preview(pending), reply_markup=build_preview_keyboard(pending_id),
            parse_mode=ParseMode.HTML,
        )

    async def add_pending(query, context, pending_id: str):
        store = _pending_store(context)
        # Remove before the API call, with no await in between: a second tap (or
        # a concurrent one) finds nothing and can never create twice.
        pending = store.pop(pending_id, None)
        if pending is None:
            await query.answer(EXPIRED_MESSAGE, show_alert=True)
            return
        if _is_expired(pending):
            await query.edit_message_text(EXPIRED_MESSAGE)
            return

        user_client = await user_client_for(query)
        if user_client is None:
            store[pending_id] = pending
            return

        payload = {
            "type": "expense",
            "account_id": pending["account_id"],
            "amount": pending["amount"],
            "description": pending["description"],
        }
        if pending.get("category_id"):
            payload["category_id"] = pending["category_id"]
        try:
            result = await user_client.create_transaction(payload)
        except httpx.HTTPError as e:
            logger.error("Quick add failed to create transaction: %s", describe_error(e))
            store[pending_id] = pending  # keep it so the user can tap Add again
            await query.answer("Failed to record that. Please try again.", show_alert=True)
            return

        created = result.get("transaction", {}) if isinstance(result, dict) else {}
        account = _account_lookup(pending["accounts"], created.get("account_id") or pending["account_id"])
        currency = account.get("currency") or pending["currency"]
        created_category = created.get("category_id") or pending.get("category_id")
        category = _category_lookup(pending["categories"], created_category)
        if not category and created_category and created_category == pending.get("suggested_id"):
            category = {"name": pending.get("category_name")}
        text = format_added_line(pending["amount"], currency, category,
                                 account.get("name") or pending["account_name"])
        text += _currency_note(pending["currency_hint"], currency)
        await query.edit_message_text(
            text,
            reply_markup=build_undo_keyboard(str(created.get("id"))) if created.get("id") else None,
            parse_mode=ParseMode.HTML,
        )

    async def on_pending_button(update, context, action: str, pending_id: str, extra: Optional[str]):
        query = update.callback_query
        store = _pending_store(context)

        if action == "pa":
            await add_pending(query, context, pending_id)
            return

        pending = store.get(pending_id)
        if pending is None:
            await query.answer(EXPIRED_MESSAGE, show_alert=True)
            return
        if _is_expired(pending):
            del store[pending_id]
            await query.edit_message_text(EXPIRED_MESSAGE)
            return

        index = int(extra) if extra and extra.isdigit() else -1
        if action == "px":
            del store[pending_id]
            await query.edit_message_text("Not added.")
        elif action == "pb":
            await show_preview(query, pending_id, pending)
        elif action in ("pc", "pcp"):
            await query.edit_message_reply_markup(reply_markup=_category_picker(
                pending_id, pending["categories"], max(index, 0), pick="psc", nav_action="pcp", back="pb"))
        elif action == "psc":
            ordered = _expense_categories(pending["categories"])
            if not 0 <= index < len(ordered):
                await query.answer("That category list changed. Try again.", show_alert=True)
                return
            pending.update(category_id=ordered[index]["id"], category_name=ordered[index]["name"],
                           suggested_id=None, explicit_category=True)
            await show_preview(query, pending_id, pending)
        elif action == "pk":
            await query.edit_message_reply_markup(reply_markup=_account_picker(
                pending_id, pending["accounts"], pick="psk", back="pb"))
        elif action == "psk":
            eligible = _eligible_accounts(pending["accounts"])
            if not 0 <= index < len(eligible):
                await query.answer("That account list changed. Try again.", show_alert=True)
                return
            account = eligible[index]
            pending.update(account_id=account["id"], account_name=account["name"],
                           currency=account.get("currency") or pending["currency"])
            if not pending.get("explicit_category"):
                # Rules can depend on the account, so look the suggestion up again.
                user_client = await user_client_for(query)
                if user_client is None:
                    return
                suggestion = await _suggest_category(user_client, pending["description"],
                                                     pending["amount"], pending["account_id"])
                pending.update(category_id=None, suggested_id=None, category_name=None)
                if suggestion:
                    pending["category_name"] = suggestion.get("category_name")
                    if suggestion.get("reason") == "history":
                        pending["category_id"] = suggestion.get("category_id")
                    else:
                        pending["suggested_id"] = suggestion.get("category_id")
            await show_preview(query, pending_id, pending)

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

        if action in PENDING_ACTIONS:
            await on_pending_button(update, context, action, transaction_id, extra)
            return

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
