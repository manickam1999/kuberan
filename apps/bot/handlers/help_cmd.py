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

<b>Quick add</b>
Just type an expense: <code>coffee 18.40</code> or <code>18.40 coffee</code>
The bot replies with a preview: nothing is saved until you tap Add
End the description with a category and/or account name to pick them (<code>18.40 coffee dining maybank</code>)
/undo - Remove the last entry you added here (within 10 minutes)
Send a photo or PDF right after an entry to attach it as a receipt

<b>Transactions</b>
/expense - Record an expense
/expense 50 Coffee - Quick expense with amount &amp; description
/income - Record income
/income 3000 Salary - Quick income with amount &amp; description

<b>Budgets</b>
/budgets - View budget status

<b>Reports</b>
/summary - Monthly income/spending summary
/spent - This month's spending by category
/owed - See who owes you money (shared expenses)

<b>Help</b>
/help - Show this help message
/start - Link your Kuberan account
/cancel - Cancel current operation

<b>Tips:</b>
- Amounts can include decimals and thousands commas (e.g., 50.50, 1,200)
- Amounts are in the account's currency (a typed RM or $ is not converted)
- Use buttons to pick category and account
- When creating a new category, you can set a parent and emoji icon
- Just type the command alone for a guided flow
"""


def handle():
    async def help_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        await update.message.reply_text(HELP_TEXT, parse_mode=ParseMode.HTML)

    return help_command
