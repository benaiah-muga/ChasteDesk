// SPDX-License-Identifier: AGPL-3.0-only
export function StatusBadge({ status }: { status: string }) {
  const cls =
    status === "resolved" || status === "closed"
      ? "badge badge-success"
      : status === "awaiting_human"
        ? "badge badge-warn"
        : "badge";
  return <span className={cls}>{status.replace(/_/g, " ")}</span>;
}
