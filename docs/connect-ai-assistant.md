# Connect an AI assistant

Kuberan speaks MCP (the Model Context Protocol), so Claude and other MCP apps
can answer questions about your money, and, only if you allow it, make changes.
You stay in control: the assistant gets exactly the permissions you tick, you
can see what it did, and you can disconnect it at any moment. Your data stays on
your server; the assistant receives only the answers to the questions it asks.

This is the user-facing guide. How the sign-in works under the hood, and the
operator checklist, are in [mcp-oauth.md](mcp-oauth.md).

## Before you start (server owner)

The server must be set up for it ([self-hosting.md](self-hosting.md)): the
`mcp.` and `auth.` hostnames reachable over HTTPS, and `NEXT_PUBLIC_MCP_URL`
set so Settings shows your connector address (it is `MCP_RESOURCE_URL` plus
`/mcp`, e.g. `https://mcp.example.com/mcp`). The public demo has no MCP.

## Add the connector

1. In Kuberan open **Settings > Connected apps > Connect an assistant** and
   copy the server address.
2. In Claude: **Settings > Connectors > Add custom connector**, give it a name
   such as "Kuberan" and paste the address. In Claude Code:
   `claude mcp add --transport http kuberan <address>`. Other MCP apps have a
   similar "add server by URL" setting.
3. Claude opens a Kuberan page in your browser. Sign in with your normal
   Kuberan email and password.
4. A consent screen lists what the app asks for. Tick what you allow and
   continue. Claude now shows Kuberan's tools.

There is nothing to paste or rotate: the assistant registers itself and gets a
short-lived token that it refreshes on its own.

Then just ask: "What did I spend on dining this month?", "How is my portfolio
doing?", "Who owes me money?"

## Read access and write access

Every permission is a separate checkbox on the consent screen.

**Read** is the default and covers viewing your accounts, transactions, budgets,
categories, investments, portfolio and net-worth history. The assistant can
look, never change.

**Write** is off unless the server owner turned it on, and even then it is a
separate "Allow changes" section, unticked by default. It lets the assistant:

| Permission | What it can do |
|---|---|
| Transactions | Add and edit transactions and transfers, categorise in bulk, record repayments of money owed to you, write off a receivable |
| Categories | Add and edit categories |
| Budgets | Add and edit budgets |

It can never delete anything, and it cannot touch accounts, investments or fixed
deposits. Well-behaved assistants ask you to confirm before each change.

### Write limits

Changes are capped even for an app you trust. The server owner sets the limits
(defaults shown), and Settings shows them next to each app:

- **Per change:** no single change may move more than `MCP_MAX_WRITE_AMOUNT`
  (default 10,000 in your reporting currency, compared after conversion; a
  currency with no exchange rate is refused).
- **Rate:** `MCP_WRITE_RATE_PER_MINUTE` (30) and `MCP_WRITE_RATE_PER_DAY` (500)
  writes per app, with a higher shared ceiling across all your apps.
- Dates must be within 10 years back and 7 days ahead.

Every change is recorded with the app that made it.

## The Connected apps screen

**Settings > Connected apps** lists every app you have connected:

- its name and where it connects from, and whether it can **Read only** or
  **Read & change**;
- the write limits, if it can change things;
- **Show its changes**, a log of what that app did, and **Recent changes by
  apps**, the same log across all apps, newest first;
- **Disconnect**.

## Disconnect and reconnect

**Disconnect** revokes the app's access immediately for changes and within about
a minute for anything already under way. The app can connect again later, and
you will be asked for permissions again. Removing the connector in Claude only
removes it on Claude's side, so also disconnect it here.

To **change permissions**, disconnect the app and add the connector again; the
consent screen reappears. This is also needed after the server owner enables
write access, because existing connections were registered as read-only.

### Apps marked "Reconnect to choose permissions"

Connections made before per-app permissions existed show this badge. They no
longer work: the assistant is refused until you reconnect once. Disconnect the
app and add it again (or let it start a new sign-in), then choose its
permissions on the consent screen.

## Privacy notes

- The assistant sees the results of the tools it calls, not your database.
  Names and descriptions in tool output are sanitised so a malicious merchant
  name cannot give the assistant instructions.
- Changing your password does not end an assistant's access; use **Disconnect**
  (or "Disconnect all apps") for that.
- Everything an assistant changes is in your audit log.
