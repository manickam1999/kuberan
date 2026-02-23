import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const BOT_USERNAME = process.env.NEXT_PUBLIC_BOT_USERNAME ?? "KuberanFinBot";

export function TelegramSetupGuide() {
  return (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle className="text-lg">Setup Guide</CardTitle>
      </CardHeader>
      <CardContent>
        <Accordion type="single" collapsible className="w-full">
          <AccordionItem value="step-1">
            <AccordionTrigger>Step 1: What is this?</AccordionTrigger>
            <AccordionContent>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>
                  The{" "}
                  <a
                    href={`https://t.me/${BOT_USERNAME}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline font-medium"
                  >
                    @{BOT_USERNAME}
                  </a>{" "}
                  Telegram bot lets you manage your finances directly from Telegram:
                </p>
                <ul className="list-disc list-inside space-y-1 ml-2">
                  <li>Check account balances with /balance</li>
                  <li>Record expenses with /expense</li>
                  <li>Track income with /income</li>
                  <li>View budget status with /budgets</li>
                  <li>Get monthly summaries with /summary</li>
                </ul>
              </div>
            </AccordionContent>
          </AccordionItem>

          <AccordionItem value="step-2">
            <AccordionTrigger>Step 2: Find the Bot</AccordionTrigger>
            <AccordionContent>
              <div className="space-y-2 text-sm text-muted-foreground">
                <ol className="list-decimal list-inside space-y-2 ml-2">
                  <li>Open Telegram on your phone or computer</li>
                  <li>
                    Click{" "}
                    <a
                      href={`https://t.me/${BOT_USERNAME}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary hover:underline font-medium"
                    >
                      @{BOT_USERNAME}
                    </a>{" "}
                    to open the bot directly
                  </li>
                  <li>Press <strong>Start</strong> or send any message to begin</li>
                </ol>
              </div>
            </AccordionContent>
          </AccordionItem>

          <AccordionItem value="step-3">
            <AccordionTrigger>Step 3: Link Your Account</AccordionTrigger>
            <AccordionContent>
              <div className="space-y-2 text-sm text-muted-foreground">
                <ol className="list-decimal list-inside space-y-2 ml-2">
                  <li>Click <strong>&quot;Generate Link Code&quot;</strong> above</li>
                  <li>Click the <strong>&quot;Open @{BOT_USERNAME} in Telegram&quot;</strong> button — this will open the bot with your code pre-filled</li>
                  <li>Alternatively, copy the command and paste it in the bot chat manually</li>
                  <li>Choose your default currency, and the bot will confirm when linking is successful</li>
                </ol>
                <p className="mt-3 text-xs">
                  ⏰ <strong>Important:</strong> Link codes expire after 15 minutes for security.
                  If your code expires, generate a new one.
                </p>
              </div>
            </AccordionContent>
          </AccordionItem>

          <AccordionItem value="step-4">
            <AccordionTrigger>Step 4: Start Using Commands</AccordionTrigger>
            <AccordionContent>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>Once linked, you can use these commands:</p>
                <ul className="list-none space-y-2 ml-2">
                  <li><code className="bg-muted px-1 py-0.5 rounded">/help</code> - See all available commands</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/balance</code> - View your account balances</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/expense 50 Coffee</code> - Record an RM50 expense</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/income 3000 Salary</code> - Record RM3000 income</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/budgets</code> - Check your budget status</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/summary</code> - Get monthly summary</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/categories</code> - View your categories</li>
                  <li><code className="bg-muted px-1 py-0.5 rounded">/clear</code> - Clear the chat</li>
                </ul>
              </div>
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      </CardContent>
    </Card>
  );
}
