from telegram import Update
from telegram.constants import ParseMode
from telegram.ext import ContextTypes

HELP_TEXT = """
<b>Kuberan Bot Commands</b>

<b>Account Management</b>
/balance - View all account balances
/accounts - List all your accounts

<b>Categories</b>
/categories - Browse your categories (with subcategories)

<b>Transactions</b>
/expense - Record an expense
/expense 50 Coffee - Quick expense with amount &amp; description
/income - Record income
/income 3000 Salary - Quick income with amount &amp; description

<b>Budgets</b>
/budgets - View budget status

<b>Reports</b>
/summary - Monthly income/spending summary
/owed - See who owes you money (shared expenses)

<b>Help</b>
/help - Show this help message
/start - Link your Kuberan account
/cancel - Cancel current operation

<b>Tips:</b>
- Amounts can include decimals (e.g., 50.50)
- Use buttons to pick category and account
- When creating a new category, you can set a parent and emoji icon
- Just type the command alone for a guided flow
"""


def handle():
    async def help_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        await update.message.reply_text(HELP_TEXT, parse_mode=ParseMode.HTML)

    return help_command
