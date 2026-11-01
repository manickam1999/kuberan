import logging
from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient
from handlers.common import describe_error, resolve_linked_user
from utils.formatting import esc

logger = logging.getLogger(__name__)


def _format_category_tree(categories: list) -> str:
    """Format categories as an indented tree with icons (HTML)."""
    parents = [c for c in categories if not c.get('parent_id')]
    children_map = {}
    for c in categories:
        pid = c.get('parent_id')
        if pid:
            children_map.setdefault(pid, []).append(c)

    lines = []
    for p in parents:
        icon = p.get('icon', '')
        prefix = f"{esc(icon)} " if icon else ""
        desc = f" - <i>{esc(p['description'])}</i>" if p.get('description') else ""
        lines.append(f"{prefix}<b>{esc(p['name'])}</b>{desc}")

        for child in children_map.get(p['id'], []):
            child_icon = child.get('icon', '')
            child_prefix = f"  {esc(child_icon)} " if child_icon else "  "
            child_desc = f" - <i>{esc(child['description'])}</i>" if child.get('description') else ""
            lines.append(f"{child_prefix}{esc(child['name'])}{child_desc}")

    return "\n".join(lines) + "\n"


def format_categories_message(categories: list) -> str:
    """Build the /categories message (HTML)."""
    expense_cats = [c for c in categories if c.get('type') == 'expense']
    income_cats = [c for c in categories if c.get('type') == 'income']

    message = "<b>Your Categories</b>\n\n"

    for type_label, type_cats in [("Expense", expense_cats), ("Income", income_cats)]:
        if not type_cats:
            continue
        message += f"<b>{type_label}</b>\n"
        message += _format_category_tree(type_cats)
        message += "\n"

    return message.strip()


def handle(api_client: KuberanAPIClient):
    async def categories_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        user_data = await resolve_linked_user(api_client, update)
        if not user_data:
            return

        try:
            user_client = api_client.for_user(user_data['auth_token'])
            categories = await user_client.get_categories()

            if not categories:
                await update.message.reply_text(
                    "You don't have any categories yet.\n"
                    "Create categories using /expense or /income, "
                    "or in the web app!"
                )
                return

            await update.message.reply_text(
                format_categories_message(categories), parse_mode=ParseMode.HTML
            )

        except Exception as e:
            logger.error("Failed to fetch categories: %s", describe_error(e))
            await update.message.reply_text(
                "Failed to fetch categories. Please try again later."
            )

    return categories_command
