from telegram import Update
from telegram.ext import ContextTypes

def handle():
    async def help_command(update: Update, context: ContextTypes.DEFAULT_TYPE):
        help_text = """
*Kuberan Bot Commands*

*Account Management*
/balance - View all account balances
/accounts - List all your accounts

*Categories*
/categories - Browse your categories (with subcategories)

*Transactions*
/expense - Record an expense
/expense 50 Coffee - Quick expense with amount & description
/income - Record income
/income 3000 Salary - Quick income with amount & description

*Budgets*
/budgets - View budget status

*Reports*
/summary - Monthly income/expense summary

*Help*
/help - Show this help message
/start - Link your Kuberan account
/cancel - Cancel current operation

*Tips:*
- Amounts can include decimals (e.g., 50.50)
- Use buttons to pick category and account
- When creating a new category, you can set a parent and emoji icon
- Just type the command alone for a guided flow
"""
        await update.message.reply_text(help_text, parse_mode='Markdown')

    return help_command
