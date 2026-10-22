// SPDX-License-Identifier: AGPL-3.0-only
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

const groups = [
  {
    title: "Everywhere",
    shortcuts: [
      ["Open search and commands", "⌘ K"],
      ["Show keyboard shortcuts", "?"],
      ["Collapse or expand the sidebar", "⌘ B"],
      ["Close the current dialog or menu", "Esc"],
    ],
  },
  {
    title: "Jump to",
    shortcuts: [
      ["Overview", "G then O"],
      ["Inbox", "G then I"],
      ["Approvals", "G then A"],
      ["Customers", "G then C"],
      ["Knowledge", "G then K"],
    ],
  },
  {
    title: "In the inbox",
    shortcuts: [
      ["Move down or up the conversation list", "J / K"],
      ["Open the selected conversation", "Enter"],
      ["Focus the reply composer", "R"],
      ["Write an internal note", "N"],
      ["Resolve or reopen the conversation", "E"],
      ["Assign the conversation to yourself", "A"],
      ["Draft with copilot", "C"],
      ["Focus conversation search", "/"],
      ["Send the reply", "⌘ Enter"],
    ],
  },
];

export function ShortcutsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>Move around the desk without reaching for the mouse.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-7 sm:grid-cols-2">
          {groups.map((group) => (
            <section key={group.title} className={group.title === "In the inbox" ? "sm:col-span-2" : ""}>
              <h3 className="eyebrow mb-2">{group.title}</h3>
              <dl className="divide-y divide-rule rounded-lg border border-rule bg-surface-2/60 px-3">
                {group.shortcuts.map(([label, keys]) => (
                  <div key={label} className="flex items-center justify-between gap-4 py-2">
                    <dt className="text-[12.5px] text-ink-2">{label}</dt>
                    <dd className="shrink-0 font-mono text-[10px] text-muted-foreground">{keys}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
