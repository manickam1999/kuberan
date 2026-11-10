import logging
from typing import Optional, Sequence, Tuple
import httpx
from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.constants import ParseMode
from telegram.ext import (
    BaseHandler, ContextTypes, ConversationHandler, CommandHandler,
    MessageHandler, CallbackQueryHandler, filters
)
from api_client import KuberanAPIClient, UserAPIClient
from handlers.common import NOT_LINKED_MESSAGE, SERVICE_UNAVAILABLE_MESSAGE, describe_error
from utils.formatting import display_icon, esc, format_currency, is_emoji_icon
from utils.parsing import amount_to_cents, parse_quick_entry  # noqa: F401 (amount_to_cents is re-exported)

logger = logging.getLogger(__name__)

AMOUNT, CATEGORY, CONFIRM, ACCOUNT, NEW_CATEGORY, NEW_ACCOUNT, \
    NEW_CAT_PARENT, NEW_CAT_ICON = range(8)

TXN_KEY = 'txn'

# Free-text steps only ever run in private chats (the bot ignores groups).
PRIVATE_TEXT = filters.ChatType.PRIVATE & filters.TEXT & ~filters.COMMAND

# Account types income and expenses are recorded against (mirrors the API's
# AccountType.IsSpending).
SPENDING_ACCOUNT_TYPES = ('cash', 'credit_card', 'debt')


def _parse_amount_description(text: str) -> Tuple[Optional[int], str]:
    """Parse '50 Coffee' or 'Coffee 50' into (amount in cents, description).

    Returns (None, "") when the text has no valid amount. A zero amount comes
    back as 0, which callers treat as invalid. The currency a user may type
    ('RM50', '$5') is ignored here: amounts are in the account's currency.
    """
    entry = parse_quick_entry(text)
    if entry is None:
        return None, ""
    return entry.amount, entry.description


def _extract_account_and_category(text: str, accounts: list, categories: list):
    """Match last words of text against account/category names (right-to-left).

    Returns (description, matched_account_dict_or_None, matched_category_dict_or_None).
    """
    if not text:
        return text, None, None

    words = text.split()
    matched_account = None
    matched_category = None

    # Check last word against account names
    if words:
        last = words[-1].lower()
        for acc in accounts:
            if acc.get('name', '').lower() == last:
                matched_account = acc
                words = words[:-1]
                break

    # Check (new) last word against category names
    if words:
        last = words[-1].lower()
        for cat in categories:
            if cat.get('name', '').lower() == last:
                matched_category = cat
                words = words[:-1]
                break

    return ' '.join(words), matched_account, matched_category


def _get_default_account(accounts: list, preferred_id: Optional[str] = None):
    """Pick the account a new transaction starts on.

    Uses the user's saved default while it is still an active spending account,
    otherwise the first active spending account.
    """
    eligible = [a for a in accounts if a.get('type') in SPENDING_ACCOUNT_TYPES]
    active = [a for a in eligible if a.get('is_active', True)]
    preferred = next((a for a in active if preferred_id and a.get('id') == preferred_id), None)
    if preferred:
        return preferred
    if active:
        return active[0]
    return eligible[0] if eligible else None


def _order_categories_hierarchically(categories: list) -> list:
    """Order categories so children appear directly after their parent."""
    parents = [c for c in categories if not c.get('parent_id')]
    children_map = {}
    for c in categories:
        pid = c.get('parent_id')
        if pid:
            children_map.setdefault(pid, []).append(c)

    ordered = []
    for p in parents:
        ordered.append(p)
        for child in children_map.get(p['id'], []):
            ordered.append(child)

    # Append any orphans whose parent isn't in the list
    seen_ids = {c['id'] for c in ordered}
    for c in categories:
        if c['id'] not in seen_ids:
            ordered.append(c)

    return ordered


def _category_button_label(cat: dict) -> str:
    """Build a button label with optional icon and subcategory indicator."""
    icon = display_icon(cat.get('icon', ''))
    name = cat['name']
    if icon:
        return f"{icon} {name}"
    if cat.get('parent_id'):
        return f"  {name}"
    return name


def _build_category_keyboard(categories: list, page: int = 0):
    """Build inline keyboard for category selection with hierarchy and icons."""
    ordered = _order_categories_hierarchically(categories)

    page_size = 9
    start = page * page_size
    page_cats = ordered[start:start + page_size]

    buttons = []
    for i in range(0, len(page_cats), 3):
        row = [
            InlineKeyboardButton(
                _category_button_label(cat),
                callback_data=f"cat:{cat['id']}"
            )
            for cat in page_cats[i:i + 3]
        ]
        buttons.append(row)

    nav_row = []
    if page > 0:
        nav_row.append(InlineKeyboardButton("< Prev", callback_data=f"cat:page:{page - 1}"))
    if start + page_size < len(ordered):
        nav_row.append(InlineKeyboardButton("Next >", callback_data=f"cat:page:{page + 1}"))
    if nav_row:
        buttons.append(nav_row)

    buttons.append([
        InlineKeyboardButton("+ New", callback_data="cat:new"),
        InlineKeyboardButton("Skip", callback_data="cat:none"),
    ])
    return InlineKeyboardMarkup(buttons)


def _build_account_keyboard(accounts: list):
    """Build inline keyboard for account selection.

    A transaction is recorded in its account's currency, so when the eligible
    accounts use more than one currency each button shows its currency.
    """
    eligible = [a for a in accounts if a.get('type') in SPENDING_ACCOUNT_TYPES]
    multi_currency = len({a.get('currency') for a in eligible}) > 1
    buttons = []
    for i in range(0, len(eligible), 2):
        row = [
            InlineKeyboardButton(
                f"{acc['name']} ({acc['currency']})" if multi_currency and acc.get('currency')
                else acc['name'],
                callback_data=f"acc:{acc['id']}",
            )
            for acc in eligible[i:i + 2]
        ]
        buttons.append(row)
    buttons.append([
        InlineKeyboardButton("+ New", callback_data="acc:new"),
        InlineKeyboardButton("Back", callback_data="acc:back"),
    ])
    return InlineKeyboardMarkup(buttons)


def _build_confirm_keyboard():
    """Build confirmation inline keyboard."""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("Change Category", callback_data="txn:chg_cat"),
            InlineKeyboardButton("Change Account", callback_data="txn:chg_acc"),
        ],
        [InlineKeyboardButton("Confirm", callback_data="txn:confirm")],
        [InlineKeyboardButton("Cancel", callback_data="txn:cancel")],
    ])


def _format_category_display(txn_data: dict) -> str:
    """Format category name with icon if available (HTML-escaped)."""
    cat = txn_data.get('category_name') or "None"
    icon = display_icon(txn_data.get('category_icon', ''))
    text = f"{icon} {cat}".strip() if icon else cat
    return esc(text)


def _format_confirm_message(txn_data: dict) -> str:
    """Format the confirmation card text (HTML)."""
    txn_type = txn_data['type'].title()
    currency = txn_data.get('currency', 'MYR')
    amount_str = esc(format_currency(txn_data['amount'], currency))
    desc = esc(txn_data['description'] or txn_type)
    cat = _format_category_display(txn_data)
    acc = esc(txn_data.get('account_name') or "Unknown")
    return (
        f"<b>{txn_type}: {amount_str}</b>\n"
        f"{desc}\n\n"
        f"Category: {cat}\n"
        f"Account: {acc}\n"
        f"Currency: {esc(currency)}"
    )


def _format_success_message(txn_data: dict) -> str:
    """Format the success message (HTML)."""
    txn_type = txn_data['type'].title()
    currency = txn_data.get('currency', 'MYR')
    amount_str = esc(format_currency(txn_data['amount'], currency))
    desc = esc(txn_data['description'] or txn_type)
    cat = _format_category_display(txn_data)
    acc = esc(txn_data.get('account_name') or "Unknown")
    return (
        f"<b>{txn_type} Recorded</b>\n\n"
        f"Amount: {amount_str}\n"
        f"Description: {desc}\n"
        f"Category: {cat}\n"
        f"Account: {acc}"
    )


def _format_category_prompt(txn_data: dict) -> str:
    """Format the 'Select a category' prompt (HTML)."""
    txn_type = txn_data['type'].title()
    amount_str = esc(format_currency(txn_data['amount'], txn_data.get('currency', 'MYR')))
    desc = esc(txn_data.get('description') or txn_type)
    return f"<b>{txn_type}: {amount_str}</b>\n{desc}\n\nSelect a category:"


async def _resolve_user_and_setup(api_client: KuberanAPIClient, telegram_user_id: int,
                                  context: ContextTypes.DEFAULT_TYPE
                                  ) -> Tuple[Optional[UserAPIClient], Optional[str]]:
    """Resolve user, fetch accounts and categories. Returns (user_client, error_msg)."""
    try:
        user_data = await api_client.resolve_user(telegram_user_id)
    except httpx.HTTPError as e:
        logger.error("Failed to resolve Telegram user: %s", describe_error(e))
        return None, SERVICE_UNAVAILABLE_MESSAGE
    if not user_data:
        return None, NOT_LINKED_MESSAGE

    await api_client.record_activity(telegram_user_id)

    user_client = api_client.for_user(user_data['auth_token'])
    try:
        accounts = await user_client.get_accounts()
    except httpx.HTTPError as e:
        logger.error("Failed to fetch accounts: %s", describe_error(e))
        return None, SERVICE_UNAVAILABLE_MESSAGE

    try:
        preferred_id = (await user_client.get_profile()).get('default_account_id')
    except httpx.HTTPError as e:
        # The default is a convenience; never block recording a transaction on it.
        logger.warning("Failed to fetch profile for default account: %s", describe_error(e))
        preferred_id = None

    default_account = _get_default_account(accounts, preferred_id)
    # Quick add records straight to the account only when the choice is not a
    # guess: the saved default, or the one and only spending account.
    active_spending = [a for a in accounts
                       if a.get('type') in SPENDING_ACCOUNT_TYPES and a.get('is_active', True)]
    default_confident = bool(default_account) and (
        len(active_spending) == 1
        or (preferred_id is not None and default_account.get('id') == preferred_id)
    )
    if not default_account:
        return None, "No active accounts found. Please create an account in the web app first."

    try:
        categories = await user_client.get_categories()
    except httpx.HTTPError as e:
        logger.error("Failed to fetch categories: %s", describe_error(e))
        return None, SERVICE_UNAVAILABLE_MESSAGE

    context.user_data[TXN_KEY] = {
        'accounts': accounts,
        'categories': categories,
        'account_id': default_account['id'],
        'account_name': default_account['name'],
        'category_id': None,
        'category_name': None,
        'category_icon': '',
        # The account decides the currency; default_currency only seeds new accounts.
        'currency': default_account.get('currency') or user_data.get('default_currency', 'MYR'),
        'user_client': user_client,
        'default_confident': default_confident,
    }

    return user_client, None


def create_transaction_conversation(api_client: KuberanAPIClient, txn_type: str,
                                    extra_entry_points: Sequence[BaseHandler] = ()
                                    ) -> ConversationHandler:
    """Factory that returns a ConversationHandler for expense or income.

    ``extra_entry_points`` are added after the command entry (the one-line
    quick add for plain text lives here so it can never run while a
    conversation is waiting for input).
    """

    async def entry_point(update: Update, context: ContextTypes.DEFAULT_TYPE):
        telegram_user_id = update.message.from_user.id

        _, error = await _resolve_user_and_setup(api_client, telegram_user_id, context)
        if error:
            await update.message.reply_text(error)
            return ConversationHandler.END

        txn = context.user_data[TXN_KEY]
        txn['type'] = txn_type

        # Check if user provided args (quick path)
        input_text = ' '.join(context.args) if context.args else ''
        amount, description = _parse_amount_description(input_text)

        if amount:
            # Smart match: check if last words are account/category names
            description, matched_acc, matched_cat = _extract_account_and_category(
                description, txn['accounts'], txn['categories']
            )
            if matched_acc:
                txn['account_id'] = matched_acc['id']
                txn['account_name'] = matched_acc['name']
            if matched_cat:
                txn['category_id'] = matched_cat['id']
                txn['category_name'] = matched_cat['name']
                txn['category_icon'] = matched_cat.get('icon', '')

            txn['amount'] = amount
            txn['description'] = description

            categories = txn['categories']
            if matched_cat or not categories:
                # Category already set or none exist — go straight to confirm
                msg = await update.message.reply_text(
                    _format_confirm_message(txn),
                    reply_markup=_build_confirm_keyboard(),
                    parse_mode=ParseMode.HTML
                )
                txn['message_id'] = msg.message_id
                return CONFIRM

            msg = await update.message.reply_text(
                _format_category_prompt(txn),
                reply_markup=_build_category_keyboard(categories),
                parse_mode=ParseMode.HTML
            )
            txn['message_id'] = msg.message_id
            return CATEGORY

        # Guided path — ask for amount
        await update.message.reply_text(
            f"How much was the {txn_type}?\n"
            f"Type the amount, or amount and description (e.g. <code>50 Coffee</code>)",
            parse_mode=ParseMode.HTML
        )
        return AMOUNT

    async def receive_amount(update: Update, context: ContextTypes.DEFAULT_TYPE):
        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await update.message.reply_text("Something went wrong. Please start over with /" + txn_type)
            return ConversationHandler.END

        amount, description = _parse_amount_description(update.message.text)
        if not amount:
            await update.message.reply_text("Please enter a valid amount (e.g. <code>50</code> or <code>50 Coffee</code>)", parse_mode=ParseMode.HTML)
            return AMOUNT

        # Smart match: check if last words are account/category names
        description, matched_acc, matched_cat = _extract_account_and_category(
            description, txn['accounts'], txn['categories']
        )
        if matched_acc:
            txn['account_id'] = matched_acc['id']
            txn['account_name'] = matched_acc['name']
        if matched_cat:
            txn['category_id'] = matched_cat['id']
            txn['category_name'] = matched_cat['name']
            txn['category_icon'] = matched_cat.get('icon', '')

        txn['amount'] = amount
        txn['description'] = description

        categories = txn['categories']
        if matched_cat or not categories:
            msg = await update.message.reply_text(
                _format_confirm_message(txn),
                reply_markup=_build_confirm_keyboard(),
                parse_mode=ParseMode.HTML
            )
            txn['message_id'] = msg.message_id
            return CONFIRM

        msg = await update.message.reply_text(
            _format_category_prompt(txn),
            reply_markup=_build_category_keyboard(categories),
            parse_mode=ParseMode.HTML
        )
        txn['message_id'] = msg.message_id
        return CATEGORY

    async def handle_category_callback(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await query.edit_message_text("Session expired. Please start over with /" + txn_type)
            return ConversationHandler.END

        data = query.data

        # Handle pagination
        if data.startswith("cat:page:"):
            page = int(data.split(":")[-1])
            await query.edit_message_text(
                _format_category_prompt(txn),
                reply_markup=_build_category_keyboard(txn['categories'], page),
                parse_mode=ParseMode.HTML
            )
            return CATEGORY

        # Create new category
        if data == "cat:new":
            await query.edit_message_text("Type a name for the new category:")
            return NEW_CATEGORY

        if data == "cat:none":
            txn['category_id'] = None
            txn['category_name'] = None
            txn['category_icon'] = ''
        else:
            cat_id = data.split(":")[1]
            txn['category_id'] = cat_id
            cat = next((c for c in txn['categories'] if str(c['id']) == cat_id), None)
            txn['category_name'] = cat['name'] if cat else None
            txn['category_icon'] = cat.get('icon', '') if cat else ''

        await query.edit_message_text(
            _format_confirm_message(txn),
            reply_markup=_build_confirm_keyboard(),
            parse_mode=ParseMode.HTML
        )
        return CONFIRM

    async def receive_new_category(update: Update, context: ContextTypes.DEFAULT_TYPE):
        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await update.message.reply_text("Something went wrong. Please start over with /" + txn_type)
            return ConversationHandler.END

        name = update.message.text.strip()
        if not name:
            await update.message.reply_text("Please type a category name:")
            return NEW_CATEGORY

        txn['new_cat_name'] = name

        # Build parent selection keyboard from existing top-level categories of same type
        top_level = [c for c in txn['categories']
                     if not c.get('parent_id') and c.get('type') == txn_type]

        buttons = []
        for i in range(0, len(top_level), 2):
            row = [
                InlineKeyboardButton(
                    f"{display_icon(c.get('icon', ''))} {c['name']}".strip(),
                    callback_data=f"ncp:{c['id']}"
                )
                for c in top_level[i:i + 2]
            ]
            buttons.append(row)
        buttons.append([
            InlineKeyboardButton("Top-level (no parent)", callback_data="ncp:none")
        ])

        msg = await update.message.reply_text(
            f"Category: <b>{esc(name)}</b>\n\nIs this a subcategory of an existing category?",
            reply_markup=InlineKeyboardMarkup(buttons),
            parse_mode=ParseMode.HTML
        )
        txn['message_id'] = msg.message_id
        return NEW_CAT_PARENT

    async def handle_new_cat_parent(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await query.edit_message_text("Session expired. Please start over with /" + txn_type)
            return ConversationHandler.END

        data = query.data
        if data == "ncp:none":
            txn['new_cat_parent_id'] = None
        else:
            txn['new_cat_parent_id'] = data.split(":")[1]

        await query.edit_message_text(
            "Send an emoji to use as the icon (e.g. ☕ 🍕 💰), or tap Skip:",
            reply_markup=InlineKeyboardMarkup([
                [InlineKeyboardButton("Skip", callback_data="nci:skip")]
            ])
        )
        return NEW_CAT_ICON

    async def _create_new_category_and_confirm(txn, reply_func):
        """Create the category via API and show confirmation."""
        user_client = txn['user_client']
        category = await user_client.create_category(
            name=txn['new_cat_name'],
            category_type=txn['type'],
            icon=txn.get('new_cat_icon', ''),
            parent_id=txn.get('new_cat_parent_id'),
        )
        txn['category_id'] = category['id']
        txn['category_name'] = category['name']
        txn['category_icon'] = category.get('icon', '')
        # Add to cached list so it appears if user goes back to category picker
        txn['categories'].append(category)

        # Clean up temp keys
        for key in ('new_cat_name', 'new_cat_parent_id', 'new_cat_icon'):
            txn.pop(key, None)

        msg = await reply_func(
            _format_confirm_message(txn),
            reply_markup=_build_confirm_keyboard(),
            parse_mode=ParseMode.HTML
        )
        if hasattr(msg, 'message_id'):
            txn['message_id'] = msg.message_id
        return CONFIRM

    async def receive_new_cat_icon(update: Update, context: ContextTypes.DEFAULT_TYPE):
        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await update.message.reply_text("Something went wrong. Please start over with /" + txn_type)
            return ConversationHandler.END

        icon = update.message.text.strip()
        if not is_emoji_icon(icon):
            # The API only accepts a single emoji here; ask again rather than
            # failing the whole category creation.
            await update.message.reply_text(
                "That isn't a single emoji. Send one emoji (e.g. ☕ 🍕 💰), or tap Skip:",
                reply_markup=InlineKeyboardMarkup([
                    [InlineKeyboardButton("Skip", callback_data="nci:skip")]
                ])
            )
            return NEW_CAT_ICON
        txn['new_cat_icon'] = icon
        return await _create_new_category_and_confirm(txn, update.message.reply_text)

    async def handle_new_cat_icon_skip(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await query.edit_message_text("Session expired. Please start over with /" + txn_type)
            return ConversationHandler.END

        txn['new_cat_icon'] = ""
        return await _create_new_category_and_confirm(txn, query.edit_message_text)

    async def handle_confirm_callback(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await query.edit_message_text("Session expired. Please start over with /" + txn_type)
            return ConversationHandler.END

        data = query.data

        if data == "txn:cancel":
            await query.edit_message_text("Cancelled.")
            context.user_data.pop(TXN_KEY, None)
            return ConversationHandler.END

        if data == "txn:chg_cat":
            await query.edit_message_text(
                _format_category_prompt(txn),
                reply_markup=_build_category_keyboard(txn['categories']),
                parse_mode=ParseMode.HTML
            )
            return CATEGORY

        if data == "txn:chg_acc":
            await query.edit_message_text(
                "Select an account:",
                reply_markup=_build_account_keyboard(txn['accounts']),
            )
            return ACCOUNT

        if data == "txn:confirm":
            user_client = txn['user_client']
            transaction_data = {
                "type": txn['type'],
                "account_id": txn['account_id'],
                "amount": txn['amount'],
                "description": txn['description'] or txn['type'].title(),
            }
            if txn.get('category_id'):
                transaction_data['category_id'] = txn['category_id']

            try:
                await user_client.create_transaction(transaction_data)
            except httpx.HTTPError as e:
                logger.error("Failed to create transaction: %s", describe_error(e))
                await query.edit_message_text(
                    "Failed to record transaction. Please try again.",
                    reply_markup=_build_confirm_keyboard(),
                )
                return CONFIRM

            await query.edit_message_text(
                _format_success_message(txn),
                parse_mode=ParseMode.HTML
            )
            context.user_data.pop(TXN_KEY, None)
            return ConversationHandler.END

        return CONFIRM

    async def handle_account_callback(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        await query.answer()

        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await query.edit_message_text("Session expired. Please start over with /" + txn_type)
            return ConversationHandler.END

        data = query.data

        if data == "acc:back":
            await query.edit_message_text(
                _format_confirm_message(txn),
                reply_markup=_build_confirm_keyboard(),
                parse_mode=ParseMode.HTML
            )
            return CONFIRM

        # Create new account
        if data == "acc:new":
            await query.edit_message_text("Type a name for the new account:")
            return NEW_ACCOUNT

        acc_id = data.split(":")[1]
        acc = next((a for a in txn['accounts'] if str(a['id']) == acc_id), None)
        txn['account_id'] = acc_id
        txn['account_name'] = acc['name'] if acc else "Unknown"
        # Amounts are recorded in the chosen account's currency.
        if acc and acc.get('currency'):
            txn['currency'] = acc['currency']

        await query.edit_message_text(
            _format_confirm_message(txn),
            reply_markup=_build_confirm_keyboard(),
            parse_mode=ParseMode.HTML
        )
        return CONFIRM

    async def receive_new_account(update: Update, context: ContextTypes.DEFAULT_TYPE):
        txn = context.user_data.get(TXN_KEY)
        if not txn:
            await update.message.reply_text("Something went wrong. Please start over with /" + txn_type)
            return ConversationHandler.END

        name = update.message.text.strip()
        if not name:
            await update.message.reply_text("Please type an account name:")
            return NEW_ACCOUNT

        user_client = txn['user_client']
        account = await user_client.create_cash_account(name, currency=txn.get('currency', ''))
        txn['account_id'] = account['id']
        txn['account_name'] = account['name']
        if account.get('currency'):
            txn['currency'] = account['currency']
        # Add to cached list so it appears if user goes back to account picker
        txn['accounts'].append(account)

        msg = await update.message.reply_text(
            _format_confirm_message(txn),
            reply_markup=_build_confirm_keyboard(),
            parse_mode=ParseMode.HTML
        )
        txn['message_id'] = msg.message_id
        return CONFIRM

    async def cancel(update: Update, context: ContextTypes.DEFAULT_TYPE):
        context.user_data.pop(TXN_KEY, None)
        await update.message.reply_text("Cancelled.")
        return ConversationHandler.END

    async def timeout(update: Update, context: ContextTypes.DEFAULT_TYPE):
        context.user_data.pop(TXN_KEY, None)

    return ConversationHandler(
        entry_points=[CommandHandler(txn_type, entry_point, filters=filters.ChatType.PRIVATE),
                      *extra_entry_points],
        states={
            AMOUNT: [MessageHandler(PRIVATE_TEXT, receive_amount)],
            CATEGORY: [CallbackQueryHandler(handle_category_callback, pattern=r'^cat:')],
            CONFIRM: [CallbackQueryHandler(handle_confirm_callback, pattern=r'^txn:')],
            ACCOUNT: [CallbackQueryHandler(handle_account_callback, pattern=r'^acc:')],
            NEW_CATEGORY: [MessageHandler(PRIVATE_TEXT, receive_new_category)],
            NEW_ACCOUNT: [MessageHandler(PRIVATE_TEXT, receive_new_account)],
            NEW_CAT_PARENT: [CallbackQueryHandler(handle_new_cat_parent, pattern=r'^ncp:')],
            NEW_CAT_ICON: [
                CallbackQueryHandler(handle_new_cat_icon_skip, pattern=r'^nci:'),
                MessageHandler(PRIVATE_TEXT, receive_new_cat_icon),
            ],
            ConversationHandler.TIMEOUT: [MessageHandler(filters.ALL, timeout)],
        },
        fallbacks=[CommandHandler('cancel', cancel, filters=filters.ChatType.PRIVATE)],
        conversation_timeout=300,
        per_message=False,
        per_chat=True,
    )
