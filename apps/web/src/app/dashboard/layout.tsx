// SPDX-License-Identifier: AGPL-3.0-only
export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <div>
      <nav
        style={{
          display: "flex",
          gap: 16,
          alignItems: "center",
          padding: "12px 24px",
          borderBottom: "1px solid var(--border)",
          background: "var(--surface)",
        }}
        aria-label="Dashboard"
      >
        <strong>ChasteDesk</strong>
        <a href="/dashboard/inbox">Inbox</a>
        <a href="/dashboard/knowledge">Knowledge</a>
        <a href="/dashboard/settings">Settings</a>
        <a href="/dashboard/analytics">Analytics</a>
        <a href="/dashboard/audit">Audit</a>
      </nav>
      <div style={{ padding: 24 }}>{children}</div>
    </div>
  );
}
