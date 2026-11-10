"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { AuthSplit } from "@/components/auth/auth-split";
import { useAuth } from "@/hooks/use-auth";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isLoading, isAuthenticated } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.replace("/");
    }
  }, [isLoading, isAuthenticated, router]);

  if (isLoading || isAuthenticated) {
    return null;
  }

  return <AuthSplit>{children}</AuthSplit>;
}
