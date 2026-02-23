import logging
from telegram import Update
from telegram.ext import ContextTypes
from api_client import KuberanAPIClient, UserAPIClient

logger = logging.getLogger(__name__)


def _format_category_tree(categories: list) -> str:
    """Format categories as an indented tree with icons."""
    parents = [c for c in categories if not c.get('parent_id')]
    children_map = {}
    for c in categories:
        pid = c.get('parent_id')
        if pid:
            children_map.setdefault(pid, []).append(c)

    lines = []
    for p in parents:
        icon = p.get('icon', '')
        prefix = f"{icon} " if icon else ""
        desc = f" - _{p['description']}_" if p.get('description') else ""
        lines.append(f"{prefix}*{p['name']}*{desc}")

        for child in children_map.get(p['id'], []):
            child_icon = child.get('icon', '')
            child_prefix = f"  {child_icon} " if child_icon else "  "
            child_desc = f" - _{child['description']}_" if child.get('description') else ""
            lines.append(f"{child_prefix}{child['name']}{child_desc}")

    return "\n".join(lines) + "\n"


def handle(api_client: KuberanAPIClient):
    async def categories_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        telegram_user_id = update.message.from_user.id

        user_data = api_client.resolve_user(telegram_user_id)
        if not user_data:
            await update.message.reply_text(
                "Your Telegram account is not linked.\nUse /start to link your account."
            )
            return

        api_client.record_activity(telegram_user_id)

        try:
            user_client = UserAPIClient(api_client.base_url, user_data['auth_token'])
            categories = user_client.get_categories()

            if not categories:
                await update.message.reply_text(
                    "You don't have any categories yet.\n"
                    "Create categories using /expense or /income, "
                    "or in the web app!"
                )
                return

            expense_cats = [c for c in categories if c.get('type') == 'expense']
            income_cats = [c for c in categories if c.get('type') == 'income']

            message = "*Your Categories*\n\n"

            for type_label, type_cats in [("Expense", expense_cats), ("Income", income_cats)]:
                if not type_cats:
                    continue
                message += f"*{type_label}*\n"
                message += _format_category_tree(type_cats)
                message += "\n"

            await update.message.reply_text(message.strip(), parse_mode='Markdown')

        except Exception as e:
            logger.error(f"Failed to fetch categories: {e}")
            await update.message.reply_text(
                "Failed to fetch categories. Please try again later."
            )

    return categories_command
