// SPDX-License-Identifier: AGPL-3.0-only
import { lazy, Suspense } from "react";
import { Navigate, Outlet, Route, Routes, useLocation } from "react-router-dom";
import { useSession } from "@/hooks/use-session";
import { BootScreen } from "@/components/boot-screen";
import { AppShell } from "@/components/app-shell";
import { PageSkeleton } from "@/components/boot-screen";

const LoginPage = lazy(() => import("@/pages/login-page").then((module) => ({ default: module.LoginPage })));
const SetupPage = lazy(() => import("@/pages/setup-page").then((module) => ({ default: module.SetupPage })));
const InvitePage = lazy(() => import("@/pages/invite-page").then((module) => ({ default: module.InvitePage })));
const OnboardingPage = lazy(() => import("@/pages/onboarding-page").then((module) => ({ default: module.OnboardingPage })));
const OverviewPage = lazy(() => import("@/pages/overview-page").then((module) => ({ default: module.OverviewPage })));
const InboxPage = lazy(() => import("@/pages/inbox-page").then((module) => ({ default: module.InboxPage })));
const ApprovalsPage = lazy(() => import("@/pages/approvals-page").then((module) => ({ default: module.ApprovalsPage })));
const CustomersPage = lazy(() => import("@/pages/customers-page").then((module) => ({ default: module.CustomersPage })));
const CustomerDetailPage = lazy(() => import("@/pages/customers-page").then((module) => ({ default: module.CustomerDetailPage })));
const KnowledgePage = lazy(() => import("@/pages/knowledge-page").then((module) => ({ default: module.KnowledgePage })));
const QuickRepliesPage = lazy(() => import("@/pages/quick-replies-page").then((module) => ({ default: module.QuickRepliesPage })));
const AnalyticsPage = lazy(() => import("@/pages/analytics-page").then((module) => ({ default: module.AnalyticsPage })));
const AuditPage = lazy(() => import("@/pages/audit-page").then((module) => ({ default: module.AuditPage })));
const SettingsPage = lazy(() => import("@/pages/settings/settings-page").then((module) => ({ default: module.SettingsPage })));
const AccountPage = lazy(() => import("@/pages/account-page").then((module) => ({ default: module.AccountPage })));
const NotFoundPage = lazy(() => import("@/pages/not-found-page").then((module) => ({ default: module.NotFoundPage })));

function SessionGate() {
  const location = useLocation();
  const session = useSession();
  if (session.isLoading) return <BootScreen />;
  if (!session.data) return <BootScreen />;
  if (session.data.setupRequired) return <Navigate to="/setup" replace />;
  if (!session.data.authenticated) {
    const next = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?next=${next}`} replace />;
  }
  return <Outlet />;
}

export function App() {
  return (
    <Suspense fallback={<PageSkeleton />}>
      <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/invite/:id" element={<InvitePage />} />
      <Route element={<SessionGate />}>
        <Route path="/onboarding" element={<OnboardingPage />} />
        <Route path="/dashboard" element={<AppShell />}>
          <Route index element={<OverviewPage />} />
          <Route path="inbox" element={<InboxPage />} />
          <Route path="inbox/:id" element={<InboxPage />} />
          <Route path="approvals" element={<ApprovalsPage />} />
          <Route path="customers" element={<CustomersPage />} />
          <Route path="customers/:id" element={<CustomerDetailPage />} />
          <Route path="knowledge" element={<KnowledgePage />} />
          <Route path="replies" element={<QuickRepliesPage />} />
          <Route path="analytics" element={<AnalyticsPage />} />
          <Route path="audit" element={<AuditPage />} />
          <Route path="settings" element={<Navigate to="/dashboard/settings/providers" replace />} />
          <Route path="settings/:section" element={<SettingsPage />} />
          <Route path="account" element={<AccountPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  );
}
