# Telegram bot

The bot (`apps/bot`, Python) lets you check balances and log expenses from a
Telegram chat. It holds no financial data and no database access: it talks to
the Kuberan api over HTTP, acting as the user whose Telegram account you linked.
It answers only in private chats; messages in groups or channels are ignored so
balances never leak to other members.

## 1. Create the bot with BotFather

1. In Telegram, open [@BotFather](https://t.me/BotFather) and send `/newbot`.
2. Pick a display name and a username ending in `bot`, e.g. `MyKuberanBot`.
3. Copy the token BotFather returns (`123456789:ABC...`). Treat it as a
   password: anyone with it controls the bot.
4. Optional: `/setdescription`, `/setuserpic` and `/setcommands` make it look
   nicer. For `/setcommands`, paste the list in [Commands](#commands).

You run your own bot; there is no shared hosted one. The web app's link buttons
use the username in `NEXT_PUBLIC_BOT_USERNAME`, so set it to yours (below).

## 2. Configure and start it

In `.env.prod` (production compose, `docker-compose.prod.yml`):

| Variable | Purpose |
|---|---|
| `TELEGRAM_BOT_TOKEN` | The token from BotFather. Only the bot container receives it. |
| `BOT_INTERNAL_SECRET` | Shared secret between the bot and the api's internal routes. `openssl rand -hex 32`. Must be set for both. |
| `NEXT_PUBLIC_BOT_USERNAME` | Your bot's username without `@` (default `KuberanFinBot`). Baked into the web build, so redeploy after changing it. |
| `LOG_LEVEL` | `INFO` (default). `DEBUG` logs Telegram user ids. |

The compose file also fixes `API_BASE_URL=http://api:8080` for the container
(`apps/bot/.env.example` lists the same variables for running it by hand).
`REMINDER_POLL_SECONDS` (default `300`, how often the bot asks for due
reminders) is read by the bot but not passed through the production compose
file; add it to the `bot` service's `environment:` to change it.

The bot refuses to start without the token and the secret, and in the
production compose it is a normal service, so set both before
`./deploy/deploy.sh`. In development it sits behind a profile:
`docker compose --profile bot up -d`.

## 3. Link your account

1. In the web app open **Settings > Telegram** and choose **Generate link code**.
2. Choose **Open @yourbot in Telegram** (or send `/start <code>` to the bot
   yourself). The code expires after 15 minutes and works once.
3. The bot asks for your **default currency** (a button for MYR, or type any
   3-letter code). This is only the default for accounts created from chat;
   totals and reports use your reporting currency from Settings.
4. The bot confirms the link, and Settings now shows the linked chat and when it
   last messaged.

To unlink, use **Unlink** in the same place. Changing your password does not log
the bot out: unlink it explicitly if a phone is lost.

## Commands

| Command | What it does |
|---|---|
| `/start <code>` | Link your account (see above). Without a code it explains how. |
| `/help` | List the commands. |
| `/balance` | Balances of all your accounts, with totals in your reporting currency. Accounts without an exchange rate are left out and listed. |
| `/accounts` | List your accounts. |
| `/categories` | Browse your categories, with subcategories. |
| `/expense` | Record an expense with a guided flow: amount, category, account, confirm. |
| `/expense 50 Coffee` | Quick path: amount and description. If the end of the text matches an account or category name, those are picked for you. |
| `/income`, `/income 3000 Salary` | The same for income. |
| `/budgets` | Budget status with progress for the current period. |
| `/summary` | This month's income and spending summary. |
| `/owed` | Who owes you money (shared expenses and loans), from the receivables summary. |
| `/cancel` | Abort the current guided flow. Guided flows time out after 5 minutes of silence. |

Tips: amounts can have decimals (`50.50`). While recording, you can create a new
category or account on the spot, including a parent category and an emoji icon.

### Reminders

Once linked, the bot messages you when a credit card or loan payment is due and
when a fixed deposit matures. The web app queues them; the bot polls and
confirms each delivery so a message is never sent twice. Turn them on and off in
**Settings > Telegram > Reminders**, or per account on its page.

### Coming in this release

<!-- Merge point: another change adds these to the bot. Keep this section only
     for what is in the release being documented, and move each item into the
     Commands table above when it ships. -->

- **Quick add:** send a plain message such as `coffee 18.40` (no command) to log
  an expense.
- **`/undo`:** remove the transaction you just added.
- **Receipt photos:** send a photo of a receipt to attach it to a transaction.
- **`/spent`:** how much you have spent, without opening the app.

## Troubleshooting

- **The bot never answers:** check `docker compose -f docker-compose.prod.yml
  logs bot`. A wrong token fails at start; a wrong `BOT_INTERNAL_SECRET` makes
  every command fail with "something went wrong".
- **"Invalid or expired link code":** generate a new code; each is single use
  and lasts 15 minutes.
- **A Telegram account can be linked to only one Kuberan user.** To move it,
  unlink it from the first user. Repeated wrong codes are temporarily refused.
- **The container shows unhealthy:** its healthcheck watches a heartbeat file
  the bot touches every 30 seconds. A stale heartbeat means polling died;
  restart the service.
