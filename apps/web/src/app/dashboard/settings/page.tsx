// SPDX-License-Identifier: AGPL-3.0-only
import ProvidersCard from "./providers-card";
import WidgetCard from "./widget-card";

export default function SettingsPage() {
  return (
    <main style={{ maxWidth: 760, margin: "0 auto", padding: 24 }}>
      <h1>Settings</h1>
      <div className="stack">
        <ProvidersCard />
        <WidgetCard />
      </div>
    </main>
  );
}
