import Link from "next/link";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 p-4 text-center">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="text-xl font-semibold">Page not found</h1>
      <p className="max-w-sm text-sm text-muted-foreground">
        The page you were looking for does not exist or has moved.
      </p>
      <Button asChild className="mt-2">
        <Link href="/">Go to dashboard</Link>
      </Button>
    </div>
  );
}
