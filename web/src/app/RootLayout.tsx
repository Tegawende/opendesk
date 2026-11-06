import { useTranslation } from "react-i18next";
import { QueryClientProvider } from "@tanstack/react-query";
import { Outlet, useMatches, useNavigate, useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";
import { ConfirmProvider } from "@/components/ui/confirm";
import { Toaster } from "@/components/ui/toast";
import { TooltipProvider } from "@/components/ui/tooltip";
import { pageview } from "@/lib/analytics";
import { queryClient } from "@/lib/queryClient";
import { onAccountFallback } from "@/lib/session";
import { useDesktopNavigation } from "./useDesktopBridge";

// A page-specific title takes precedence while translated route titles refresh.
let routeTitle = "opendesk";
let pageTitle: string | null = null;

function useRouteTitle() {
  const { t } = useTranslation("navigation");
  const matches = useMatches();
  const title = [...matches].reverse().find((m) => m.staticData?.title)?.staticData.title;
  const translatedTitle = title ? t(`titles.${title}`) : null;
  useEffect(() => {
    routeTitle = translatedTitle ? `${translatedTitle} · opendesk` : "opendesk";
    if (pageTitle === null) document.title = routeTitle;
  }, [translatedTitle]);
}

function usePageviews() {
  const path = useRouterState({ select: (s) => s.location.pathname });
  useEffect(() => pageview(path), [path]);
}

/**
 * When the active account's session ends on its own (revoked elsewhere, expired),
 * another signed-in account takes over: start it fresh from its inbox, or go to
 * sign-in when none is left.
 */
function useAccountFallback() {
  const navigate = useNavigate();
  useEffect(
    () =>
      onAccountFallback((s) => {
        if (s.status === "authed") void navigate({ to: s.workspaceId ? "/" : "/onboarding" });
        else void navigate({ to: "/login", search: { next: location.pathname + location.search } });
      }),
    [navigate],
  );
}

export function RootLayout() {
  useRouteTitle();
  usePageviews();
  useAccountFallback();
  useDesktopNavigation();
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={200} skipDelayDuration={300}>
        <ConfirmProvider>
          <Outlet />
          <Toaster />
        </ConfirmProvider>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

/**
 * Set a page-specific tab title (a ticket subject, an article). bare: the title alone, without
 * the brand (a page to print, whose title names the saved file).
 */
export function useDocumentTitle(title: string | null | undefined, { bare = false }: { bare?: boolean } = {}) {
  useEffect(() => {
    if (!title) return;
    pageTitle = title;
    document.title = bare ? title : `${title} · opendesk`;
    return () => {
      pageTitle = null;
      document.title = routeTitle;
    };
  }, [title, bare]);
}
