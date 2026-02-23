"use client";

import { useState, useEffect } from "react";
import { toast } from "sonner";
import { useTelegramLink, useGenerateLinkCode, useUnlinkTelegram } from "@/hooks/use-telegram";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { TelegramSetupGuide } from "./telegram-setup-guide";
import { Copy, Check, Unlink, RefreshCw, ExternalLink } from "lucide-react";

const BOT_USERNAME = process.env.NEXT_PUBLIC_BOT_USERNAME ?? "KuberanFinBot";

export function TelegramSettings() {
  const { data, isLoading, refetch } = useTelegramLink();
  const generateCode = useGenerateLinkCode();
  const unlinkMutation = useUnlinkTelegram();

  const [linkCode, setLinkCode] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [showGuide, setShowGuide] = useState(false);

  const link = data?.link;
  const isLinked = link && link.telegram_user_id > 0 && link.is_active;
  const isPending = !isLinked && linkCode !== null;

  useEffect(() => {
    if (isLinked) {
      setLinkCode(null);
    }
  }, [isLinked]);

  const handleGenerateCode = async () => {
    try {
      const result = await generateCode.mutateAsync();
      setLinkCode(result.link_code);
      setShowGuide(true);
      toast.success("Link code generated", {
        description: "Copy the code and send it to your Telegram bot with /start",
      });
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : "Please try again";
      toast.error("Failed to generate code", { description: message });
    }
  };

  const handleCopyCode = async () => {
    if (linkCode) {
      await navigator.clipboard.writeText(`/start ${linkCode}`);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
      toast.success("Code copied!", {
        description: "Now send /start CODE to your Telegram bot",
      });
    }
  };

  const handleUnlink = async () => {
    if (!confirm("Are you sure you want to unlink your Telegram account?")) {
      return;
    }

    try {
      await unlinkMutation.mutateAsync();
      setLinkCode(null);
      toast.success("Telegram unlinked", {
        description: "Your Telegram account has been disconnected",
      });
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : "Please try again";
      toast.error("Failed to unlink", { description: message });
    }
  };

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Telegram Bot</CardTitle>
          <CardDescription>Loading...</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Telegram Bot</CardTitle>
              <CardDescription>
                Control your finances via Telegram
              </CardDescription>
            </div>
            {isLinked && (
              <Badge variant="default" className="bg-green-600">
                Linked
              </Badge>
            )}
            {isPending && (
              <Badge variant="secondary">
                Pending
              </Badge>
            )}
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          {!isLinked && !isPending && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Link your Telegram account to manage your finances from anywhere via{" "}
                <a
                  href={`https://t.me/${BOT_USERNAME}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary hover:underline font-medium"
                >
                  @{BOT_USERNAME}
                </a>.
              </p>
              <div className="flex gap-2">
                <Button
                  onClick={handleGenerateCode}
                  disabled={generateCode.isPending}
                >
                  {generateCode.isPending && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}
                  Generate Link Code
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowGuide(!showGuide)}
                >
                  {showGuide ? "Hide" : "Show"} Setup Guide
                </Button>
              </div>
            </div>
          )}

          {isPending && linkCode && (
            <Alert>
              <AlertDescription className="space-y-3">
                <div>
                  <strong className="text-sm">Link your Telegram account:</strong>
                  <p className="text-xs text-muted-foreground mt-1">
                    Click the link below to open the bot with your code, or copy the command manually.
                  </p>
                </div>
                <a
                  href={`https://t.me/${BOT_USERNAME}?start=${linkCode}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 bg-primary text-primary-foreground px-4 py-2.5 rounded-md text-sm font-medium hover:bg-primary/90 transition-colors w-fit"
                >
                  <ExternalLink className="h-4 w-4" />
                  Open @{BOT_USERNAME} in Telegram
                </a>
                <div className="flex items-center gap-2">
                  <code className="flex-1 bg-muted px-3 py-2 rounded text-sm font-mono">
                    /start {linkCode}
                  </code>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={handleCopyCode}
                  >
                    {copied ? (
                      <Check className="h-4 w-4" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Code expires in 15 minutes. Not working?{" "}
                  <button
                    onClick={handleGenerateCode}
                    className="text-primary hover:underline"
                  >
                    Generate a new code
                  </button>
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => refetch()}
                >
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Check Status
                </Button>
              </AlertDescription>
            </Alert>
          )}

          {isLinked && (
            <div className="space-y-4">
              <Alert className="border-green-200 bg-green-50">
                <AlertDescription className="text-green-900">
                  <strong>✅ Linked successfully!</strong>
                  <div className="mt-2 text-sm">
                    <p>Telegram: <strong>@{link.telegram_username || "Unknown"}</strong></p>
                    {link.telegram_first_name && (
                      <p>Name: {link.telegram_first_name}</p>
                    )}
                    {link.message_count > 0 && (
                      <p className="text-xs text-muted-foreground mt-1">
                        {link.message_count} message{link.message_count !== 1 ? "s" : ""} sent
                      </p>
                    )}
                  </div>
                </AlertDescription>
              </Alert>

              <div className="flex gap-2">
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={handleUnlink}
                  disabled={unlinkMutation.isPending}
                >
                  <Unlink className="mr-2 h-4 w-4" />
                  Unlink Account
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowGuide(!showGuide)}
                >
                  {showGuide ? "Hide" : "Show"} Command Guide
                </Button>
              </div>
            </div>
          )}

        </CardContent>
      </Card>

      {showGuide && <TelegramSetupGuide />}
    </div>
  );
}
