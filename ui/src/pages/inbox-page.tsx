// SPDX-License-Identifier: AGPL-3.0-only
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useOutletContext, useParams, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowLeft,
  BadgeCheck,
  BookOpen,
  ClockAlert,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Copy,
  Inbox,
  Info,
  ListFilter,
  Lock,
  MessageSquareText,
  MoreHorizontal,
  NotebookPen,
  RefreshCw,
  RotateCcw,
  Search,
  Send,
  SlidersHorizontal,
  Sparkles,
  Tag,
  UserRound,
  Users,
  X,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import type { AppSession, ConversationDetail, ConversationListItem, Message } from "@/lib/types";
import { api } from "@/lib/api";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { avatarTone, cn } from "@/lib/utils";
import { copyText, dateTime, relativeTime, titleCase, truncate } from "@/lib/format";
import { isSnoozed, slaState, snoozeChoices, snoozeLabel, ticketPriorities } from "@/lib/tickets";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { StatusBadge, statusDotClass } from "@/components/status-badge";
import { CopyButton } from "@/components/copy-button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { QuickReplyPicker } from "@/components/quick-reply-picker";
import { PriorityBadge } from "@/components/priority-badge";

type ShellContext = { session: AppSession };

const statusFilters = [
  { key: "", label: "All" },
  { key: "open", label: "Open" },
  { key: "ai_active", label: "AI" },
  { key: "awaiting_human", label: "Awaiting" },
  { key: "human_active", label: "Human" },
  { key: "snoozed", label: "Snoozed" },
  { key: "resolved", label: "Resolved" },
  { key: "closed", label: "Closed" },
];

function conversationHref(item: ConversationListItem, search: URLSearchParams) {
  const params = new URLSearchParams(search);
  return `/dashboard/inbox/${item.id}${params.toString() ? `?${params.toString()}` : ""}`;
}

function ConversationRow({
  item,
  active,
  selectMode,
  selected,
  onSelect,
  search,
}: {
  item: ConversationListItem;
  active: boolean;
  selectMode: boolean;
  selected: boolean;
  onSelect: (checked: boolean) => void;
  search: URLSearchParams;
}) {
  const initials = item.customer.initial || item.customer.name.slice(0, 2).toUpperCase();
  return (
    <div className={cn("group relative border-b border-rule", selected && "bg-surface-3")}>
      <Link
        to={conversationHref(item, search)}
        aria-current={active ? "page" : undefined}
        className={cn(
          "flex items-start gap-3 py-3 pr-3 transition-colors",
          selectMode ? "pl-11" : "pl-4",
          active ? "bg-surface-2" : "hover:bg-surface-2/65",
        )}
      >
        <span className={cn("absolute bottom-0 left-0 top-0 w-[2px] bg-accent opacity-0 transition-opacity", active && "opacity-100")} aria-hidden="true" />
        {!selectMode ? (
          <Avatar className="mt-0.5 size-8">
            <AvatarFallback className={avatarTone(item.customer.name)}>{initials}</AvatarFallback>
          </Avatar>
        ) : null}
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="truncate text-[12.5px] font-semibold text-ink">{item.customer.name}</span>
            <span className={cn("size-1.5 shrink-0 rounded-full", statusDotClass(item.status))} />
            <span className="ml-auto shrink-0 font-mono text-[9px] tabular text-subtle">{relativeTime(item.updatedAt)}</span>
          </span>
          <span className="mt-1 block truncate text-[11.5px] leading-[1.45] text-muted-foreground">{item.preview}</span>
          <span className="mt-1.5 flex items-center gap-x-2 gap-y-1 overflow-hidden font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">
            <span className="shrink-0 text-ink-2">#{item.ticketNumber}</span>
            <span className="shrink-0 text-subtle">/</span>
            <span className="shrink-0">{item.channelLabel}</span>
            {item.priority !== "normal" ? <PriorityBadge priority={item.priority} compact className="shrink-0" /> : null}
            {item.tags[0] ? <span className="truncate">#{item.tags[0]}</span> : null}
            {item.assigned ? <span className="hidden truncate sm:inline">{item.assigned.name}</span> : null}
            {item.slaBreached ? (
              <span className="ml-auto flex shrink-0 items-center gap-1 text-[var(--color-danger)]"><ClockAlert className="size-3" /> SLA</span>
            ) : isSnoozed(item.snoozedUntil) ? (
              <span className="ml-auto flex shrink-0 items-center gap-1 text-[var(--color-warn)]"><Clock3 className="size-3" /> snoozed</span>
            ) : item.pendingApprovals > 0 ? (
              <span className="ml-auto flex shrink-0 items-center gap-1 text-[var(--color-warn)]">
                <BadgeCheck className="size-3" /> {item.pendingApprovals}
              </span>
            ) : null}
          </span>
        </span>
      </Link>
      {selectMode ? (
        <div className="absolute left-3 top-4 z-10">
          <Checkbox checked={selected} onCheckedChange={(value) => onSelect(value === true)} aria-label={`Select ${item.customer.name}`} />
        </div>
      ) : null}
    </div>
  );
}

function MessageBubble({ message, customerName }: { message: Message; customerName: string }) {
  const isCustomer = message.role === "customer";
  const isAI = message.role === "agent_ai";
  const isNote = message.role === "internal_note";
  const isSystem = message.role === "system";
  if (isSystem) {
    return (
      <div className="flex items-center gap-3 py-1 text-[10.5px] text-subtle">
        <span className="h-px flex-1 bg-rule" />
        <span className="max-w-[80%] text-center">{message.content}</span>
        <span className="h-px flex-1 bg-rule" />
      </div>
    );
  }
  return (
    <div className={cn("flex gap-3", isCustomer ? "pr-8" : "pl-8")}>
      {isCustomer ? (
        <Avatar className="mt-0.5 size-7">
          <AvatarFallback className={avatarTone(customerName)}>{customerName.slice(0, 1).toUpperCase()}</AvatarFallback>
        </Avatar>
      ) : (
        <span className={cn("mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border", isAI ? "border-[var(--color-sidebar-rule)] bg-[var(--color-sidebar)] text-[var(--color-sidebar-ink)]" : isNote ? "border-[var(--color-info-rule)] bg-[var(--color-info-soft)] text-[var(--color-info)]" : "border-ink bg-ink text-paper")}>
          {isAI ? <Sparkles className="size-3.5" /> : isNote ? <Lock className="size-3.5" /> : <UserRound className="size-3.5" />}
        </span>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[11.5px] font-semibold text-ink">
            {isCustomer ? customerName : isAI ? "Desk AI" : isNote ? "Internal note" : "You"}
          </span>
          {isAI ? <Badge variant="info">Grounded</Badge> : null}
          {isNote ? <Badge variant="outline">Private</Badge> : null}
          <span className="ml-auto font-mono text-[9px] tabular text-subtle">{relativeTime(message.createdAt)}</span>
        </div>
        <div
          className={cn(
            "mt-1.5 rounded-lg border px-3.5 py-3 text-[13px] leading-6",
            isCustomer && "border-rule bg-surface",
            isAI && "border-rule border-l-2 border-l-[var(--color-sidebar-3)] bg-surface-2/65",
            isNote && "border-[var(--color-info-rule)] bg-[var(--color-info-soft)]/45",
            !isCustomer && !isAI && !isNote && "border-rule bg-surface-2",
          )}
        >
          <p className="chat-copy">{message.content}</p>
          {message.citations && message.citations.length > 0 ? (
            <div className="mt-3 border-t border-rule pt-2.5">
              <details className="group">
                <summary className="flex cursor-pointer list-none items-center gap-1.5 font-mono text-[9px] font-semibold uppercase tracking-[0.08em] text-ink-2 [&::-webkit-details-marker]:hidden">
                  <BookOpen className="size-3" />
                  {message.citations.length} {message.citations.length === 1 ? "source" : "sources"} behind this reply
                  <ChevronDown className="size-3 transition-transform group-open:rotate-180" />
                </summary>
                <div className="mt-2 space-y-2">
                  {message.citations.map((citation, index) => (
                    <div key={`${citation.sourceId ?? index}-${index}`} className="rounded-md border border-rule bg-surface/80 px-2.5 py-2">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-[9px] text-subtle">E{index + 1}</span>
                        <span className="text-[11px] font-semibold text-ink">{citation.sourceName ?? "Knowledge source"}</span>
                        {citation.version ? <span className="font-mono text-[9px] text-subtle">v{citation.version}</span> : null}
                      </div>
                      {citation.quote ? <p className="mt-1 text-[11px] leading-5 text-muted-foreground">“{truncate(citation.quote, 260)}”</p> : null}
                    </div>
                  ))}
                </div>
              </details>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function HandoffCard({ detail }: { detail: ConversationDetail }) {
  const handoff = detail.conversation.handoff;
  if (!handoff) return null;
  return (
    <section className="rounded-lg border border-rule border-l-2 border-l-[var(--color-warn)] bg-surface p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full border border-[var(--color-warn-rule)] bg-surface-2 text-[var(--color-warn)]"><Zap className="size-3.5" /></span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="eyebrow text-[var(--color-warn)]">Why the AI escalated</span>
            <span className="font-mono text-[9px] text-subtle">{relativeTime(detail.conversation.updatedAt)}</span>
          </div>
          <h3 className="mt-1 font-display text-[21px] leading-snug text-ink">{handoff.problem || "The agent stopped for a human."}</h3>
          {handoff.recommendedNextStep ? (
            <p className="mt-2 text-[12.5px] leading-5 text-ink-2">
              <span className="font-semibold">Recommended next step.</span> {handoff.recommendedNextStep}
            </p>
          ) : null}
          {(handoff.investigated?.length || handoff.evidence?.length) ? (
            <details className="group mt-3 border-t border-rule pt-2.5">
              <summary className="flex cursor-pointer list-none items-center gap-1.5 py-1 font-mono text-[9px] font-semibold uppercase tracking-[0.08em] text-[var(--color-warn)] [&::-webkit-details-marker]:hidden">
                What the AI checked
                <span className="font-normal text-subtle">({(handoff.investigated?.length ?? 0) + (handoff.evidence?.length ?? 0)})</span>
                <ChevronDown className="ml-auto size-3 transition-transform group-open:rotate-180" />
              </summary>
              <div className="mt-2 grid gap-3 sm:grid-cols-2">
                {handoff.investigated && handoff.investigated.length > 0 ? (
                  <div>
                    <p className="eyebrow">Checked</p>
                    <ul className="mt-1.5 space-y-1">
                      {handoff.investigated.map((item) => <li key={item} className="flex gap-1.5 text-[11px] leading-5 text-muted-foreground"><Check className="mt-1 size-3 shrink-0 text-[var(--color-success)]" />{item}</li>)}
                    </ul>
                  </div>
                ) : null}
                {handoff.evidence && handoff.evidence.length > 0 ? (
                  <div>
                    <p className="eyebrow">Evidence</p>
                    <ul className="mt-1.5 space-y-1.5">
                      {handoff.evidence.map((item, index) => (
                        <li key={index} className="text-[11px] leading-5 text-muted-foreground">
                          {typeof item === "string" ? item : (
                            <>
                              <span className="font-semibold text-ink">{item.source}</span> <span className="font-mono text-[9px] text-subtle">v{item.version}</span>
                              {item.quote ? <span className="block">“{truncate(item.quote, 180)}”</span> : null}
                            </>
                          )}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </div>
            </details>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function PendingApprovalCard({
  action,
  onDecide,
}: {
  action: ConversationDetail["actions"][number];
  onDecide: (action: ConversationDetail["actions"][number], decision: "approve" | "reject") => void;
}) {
  return (
    <section className="rounded-lg border border-rule border-l-2 border-l-[var(--color-warn)] bg-surface">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-rule bg-surface-2/60 px-3.5 py-2.5">
        <div className="flex items-center gap-2">
          <BadgeCheck className="size-3.5 text-[var(--color-warn)]" />
          <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.07em] text-[var(--color-warn)]">Approval waiting</span>
          <span className="font-mono text-[10px] text-muted-foreground">{action.toolId}</span>
        </div>
        <span className="font-mono text-[9px] text-subtle">{relativeTime(action.createdAt)}</span>
      </div>
      <div className="p-3.5">
        <pre tabIndex={0} aria-label="Action payload" className="max-h-40 overflow-auto rounded-md border border-rule bg-surface-2/60 p-2.5 font-mono text-[10.5px] leading-5 text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{JSON.stringify(action.args, null, 2)}</pre>
        <div className="mt-3 flex items-center justify-between gap-2">
          <span className="text-[10.5px] text-muted-foreground">Runs once, then verifies the result.</span>
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={() => onDecide(action, "reject")}>Reject</Button>
            <Button size="sm" onClick={() => onDecide(action, "approve")}><Check /> Approve</Button>
          </div>
        </div>
      </div>
    </section>
  );
}

function Composer({
  conversationId,
  status,
  onSent,
  copilotAvailable,
}: {
  conversationId: string;
  status: string;
  onSent: () => void;
  copilotAvailable: boolean;
}) {
  const storageKey = `chastedesk-draft:${conversationId}`;
  const [mode, setMode] = useState<"reply" | "note">("reply");
  const [value, setValue] = useState(() => {
    try { return localStorage.getItem(storageKey) ?? ""; } catch { return ""; }
  });
  const [saved, setSaved] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      try {
        if (value.trim()) localStorage.setItem(storageKey, value);
        else localStorage.removeItem(storageKey);
        setSaved(value.trim().length > 0);
      } catch {
        // Drafts are a convenience, not a requirement.
      }
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [storageKey, value]);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(200, Math.max(84, textarea.scrollHeight + 2))}px`;
  }, [value]);

  const send = useMutation({
    mutationFn: () => api.reply(conversationId, value.trim(), mode === "note"),
    onSuccess: () => {
      toast.success(mode === "note" ? "Internal note added." : "Reply sent.");
      setValue("");
      try { localStorage.removeItem(storageKey); } catch { /* optional */ }
      setSaved(false);
      void queryClient.invalidateQueries({ queryKey: ["conversation", conversationId] });
      void queryClient.invalidateQueries({ queryKey: ["inbox"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      window.setTimeout(() => textareaRef.current?.focus(), 20);
      onSent();
    },
    onError: (error) => {
      toast.error("The message did not send", { description: error instanceof Error ? error.message : undefined });
    },
  });

  const copilot = useMutation({
    mutationFn: () => api.copilotDraft(conversationId),
    onSuccess: (data) => {
      setMode("reply");
      setValue(data.draft);
      window.setTimeout(() => textareaRef.current?.focus(), 20);
      toast.success(data.grounded ? "Draft ready from reviewed sources." : "Draft ready. No confident source was found.", {
        description: data.grounded ? `${data.citations.length} cited ${data.citations.length === 1 ? "source" : "sources"}. Review before sending.` : "Check the draft carefully before sending.",
      });
    },
    onError: (error) => toast.error("Copilot could not draft a reply", { description: error instanceof Error ? error.message : undefined }),
  });

  function submit() {
    if (!value.trim() || send.isPending) return;
    send.mutate();
  }

  const resolved = status === "resolved" || status === "closed";

  return (
    <div className="safe-bottom shrink-0 border-t border-rule bg-surface px-3 pb-3 pt-2.5 sm:px-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="inline-flex h-7 items-center gap-0.5 rounded-md border border-rule bg-surface-2 p-0.5">
          <button type="button" onClick={() => setMode("reply")} className={cn("flex h-6 items-center gap-1.5 rounded-sm px-2.5 text-[11px] font-medium transition-colors", mode === "reply" ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>
            <MessageSquareText className="size-3" /> Reply
          </button>
          <button type="button" onClick={() => setMode("note")} className={cn("flex h-6 items-center gap-1.5 rounded-sm px-2.5 text-[11px] font-medium transition-colors", mode === "note" ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>
            <Lock className="size-3" /> Internal note
          </button>
        </div>
        <div className="flex items-center gap-2">
          {resolved ? <span className="hidden font-mono text-[9px] uppercase tracking-[0.07em] text-subtle sm:inline">Sending reopens</span> : null}
          {saved ? <span className="font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">Draft saved</span> : null}
        </div>
      </div>
      <div className="rounded-lg border border-rule-strong bg-surface focus-within:ring-2 focus-within:ring-ring/20">
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
              event.preventDefault();
              submit();
            }
          }}
          placeholder={mode === "note" ? "Write an internal note. The customer never sees this." : "Write a reply to the customer..."}
          maxLength={8000}
          className="block min-h-[72px] w-full resize-none bg-transparent px-3.5 py-3 text-[13px] leading-6 text-ink outline-none placeholder:text-subtle sm:min-h-[84px]"
          aria-label={mode === "note" ? "Internal note" : "Reply to customer"}
          data-composer-input
        />
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-rule px-2 py-1.5">
          <div className="flex items-center gap-1.5">
            {copilotAvailable ? (
              <Button type="button" variant="ghost" size="sm" onClick={() => copilot.mutate()} loading={copilot.isPending}>
                <Sparkles /> Draft with copilot
              </Button>
            ) : null}
            <QuickReplyPicker
              onInsert={(body) => {
                setMode("reply");
                setValue((current) => (current.trim() ? `${current.trimEnd()}\n\n${body}` : body));
                window.setTimeout(() => textareaRef.current?.focus(), 20);
              }}
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="hidden font-mono text-[9px] text-subtle sm:inline">⌘ + Enter to send</span>
            <Button type="button" size="sm" onClick={submit} loading={send.isPending} disabled={!value.trim()}>
              {mode === "note" ? <NotebookPen /> : <Send />}
              {mode === "note" ? "Add note" : "Send reply"}
            </Button>
          </div>
        </div>
      </div>
      <p className="mt-1.5 flex items-center justify-between gap-2 font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle">
        <span>{mode === "note" ? "Internal only, never quoted to the customer" : "Replies are delivered to the conversation channel"}</span>
        <span className="tabular">{value.length}/8000</span>
      </p>
    </div>
  );
}

function ConversationContextPanel({
  current,
  assignedName,
  onPriorityChange,
  onTagsChange,
  onSnooze,
  onClose,
  ticketUpdating = false,
}: {
  current: ConversationDetail;
  assignedName?: string;
  onPriorityChange?: (priority: string) => void;
  onTagsChange?: (tags: string[]) => void;
  onSnooze?: (until: string | null) => void;
  onClose?: () => void;
  ticketUpdating?: boolean;
}) {
  const [tagInput, setTagInput] = useState("");
  const [tagError, setTagError] = useState("");
  const tags = current.conversation.tags ?? [];
  const ticketClosed = current.conversation.status === "resolved" || current.conversation.status === "closed";
  const snoozed = isSnoozed(current.conversation.snoozedUntil);

  function addTag() {
    const value = tagInput.trim().replace(/^#/, "");
    if (!value) return;
    if (value.length > 24) {
      setTagError("Tags are 24 characters or fewer.");
      return;
    }
    if (tags.some((tag) => tag.toLowerCase() === value.toLowerCase())) {
      setTagInput("");
      return;
    }
    if (tags.length >= 8) {
      setTagError("Eight tags is the maximum.");
      return;
    }
    setTagError("");
    onTagsChange?.([...tags, value]);
    setTagInput("");
  }

  return (
    <>
      <div className="shrink-0 border-b border-rule px-4 py-3">
        <span className="eyebrow">Customer</span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="border-b border-rule px-4 py-4">
          <div className="flex items-center gap-3">
            <Avatar className="size-10"><AvatarFallback className={avatarTone(current.customer.name)}>{current.customer.name?.slice(0, 1).toUpperCase() || "?"}</AvatarFallback></Avatar>
            <div className="min-w-0">
              <p className="truncate text-[13px] font-semibold">{current.customer.name || "Visitor"}</p>
              <p className="truncate text-[11px] text-muted-foreground">{current.customer.email}</p>
            </div>
            <CopyButton value={current.customer.email} iconOnly className="ml-auto border-0 bg-transparent" />
          </div>
          <div className="mt-3 grid grid-cols-2 gap-2">
            {Object.entries(current.customer.attributes).slice(0, 6).map(([key, value]) => (
              <div key={key} className="rounded-md border border-rule bg-surface-2/55 px-2 py-1.5">
                <p className="font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle">{key}</p>
                <p className="mt-0.5 truncate text-[11.5px] font-medium">{String(value)}</p>
              </div>
            ))}
          </div>
          <div className="mt-3 flex items-center gap-2 text-[10.5px] text-muted-foreground">
            <Info className="size-3.5 shrink-0" />
            {current.openConversations} other open {current.openConversations === 1 ? "conversation" : "conversations"}
          </div>
        </div>
        <div className="border-b border-rule px-4 py-4">
          <div className="flex items-center justify-between">
            <span className="eyebrow">Ticket</span>
            <span className="font-mono text-[10px] text-subtle">#{current.conversation.ticketNumber}</span>
          </div>
          <div className="mt-3 space-y-3">
            <div className="flex items-center justify-between gap-3">
              <span className="text-[11px] text-muted-foreground">Priority</span>
              <Select value={current.conversation.priority} onValueChange={(value) => onPriorityChange?.(value)} disabled={ticketUpdating}>
                <SelectTrigger className="h-7 w-[8.25rem] text-[10.5px]" aria-label="Ticket priority"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {ticketPriorities.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div>
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-muted-foreground">Tags</span>
                <span className="font-mono text-[8.5px] text-subtle">{tags.length}/8</span>
              </div>
              <div className="mt-1.5 flex flex-wrap items-center gap-1">
                {tags.map((tag) => (
                  <span key={tag} className="inline-flex h-5 items-center gap-0.5 rounded-md border border-rule bg-surface-2/65 pl-1.5 pr-0.5 font-mono text-[9px] text-ink-2">
                    #{tag}
                    <button type="button" onClick={() => onTagsChange?.(tags.filter((value) => value !== tag))} disabled={ticketUpdating} className="flex size-4 items-center justify-center rounded text-subtle hover:bg-surface-3 hover:text-ink disabled:opacity-40" aria-label={`Remove tag ${tag}`}><X className="size-2.5" /></button>
                  </span>
                ))}
                {tags.length < 8 ? (
                  <input
                    value={tagInput}
                    onChange={(event) => { setTagInput(event.target.value); setTagError(""); }}
                    onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); addTag(); } }}
                    onBlur={addTag}
                    placeholder="+ tag"
                    aria-label="Add a tag"
                    className="h-5 w-16 rounded-md border border-dashed border-rule-strong bg-transparent px-1.5 font-mono text-[9px] text-ink-2 outline-none placeholder:text-subtle focus:border-solid focus:border-ink"
                    maxLength={24}
                  />
                ) : null}
              </div>
              {tagError ? <p className="mt-1 text-[9.5px] text-[var(--color-danger)]">{tagError}</p> : null}
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-[11px] text-muted-foreground">Snooze</span>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button type="button" disabled={ticketUpdating} aria-label={snoozed ? "Snooze is set. Change snooze" : "Set a snooze"} className={cn("flex h-7 items-center gap-1.5 rounded-md border border-rule bg-surface px-2 text-[10px] font-medium transition-colors hover:border-rule-strong disabled:opacity-50", snoozed ? "text-[var(--color-warn)]" : "text-muted-foreground")}>
                    <Clock3 className="size-3" /> {snoozed ? snoozeLabel(current.conversation.snoozedUntil) : "None"}
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-48">
                  <DropdownMenuLabel>Remind me later</DropdownMenuLabel>
                  {snoozeChoices().map((choice) => (
                    <DropdownMenuItem key={choice.label} onClick={() => onSnooze?.(choice.until.toISOString())}><Clock3 /> {choice.label}</DropdownMenuItem>
                  ))}
                  {snoozed ? <><DropdownMenuSeparator /><DropdownMenuItem onClick={() => onSnooze?.(null)}><X /> Clear snooze</DropdownMenuItem></> : null}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            <div className="grid grid-cols-2 gap-2 border-t border-rule pt-2.5">
              <div className="rounded-md border border-rule bg-surface-2/45 px-2 py-1.5">
                <p className="font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">First reply</p>
                <p className={cn("mt-0.5 text-[10.5px] font-medium", slaTone(slaState(current.conversation.firstResponseDueAt, current.conversation.firstResponseAt).tone))}>{slaState(current.conversation.firstResponseDueAt, current.conversation.firstResponseAt).label}</p>
              </div>
              <div className="rounded-md border border-rule bg-surface-2/45 px-2 py-1.5">
                <p className="font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">Resolution</p>
                <p className={cn("mt-0.5 text-[10.5px] font-medium", slaTone(slaState(current.conversation.resolutionDueAt, current.conversation.resolvedAt ?? current.conversation.closedAt).tone))}>{slaState(current.conversation.resolutionDueAt, current.conversation.resolvedAt ?? current.conversation.closedAt).label}</p>
              </div>
            </div>
            <div className="flex items-center justify-between gap-2 border-t border-rule pt-2.5">
              <StatusBadge status={current.conversation.status} label={titleCase(current.conversation.status)} />
              {!ticketClosed && onClose ? (
                <button type="button" onClick={onClose} disabled={ticketUpdating} className="flex items-center gap-1 text-[10.5px] font-semibold text-muted-foreground transition-colors hover:text-[var(--color-danger)] disabled:opacity-50"><X className="size-3" /> Close ticket</button>
              ) : null}
            </div>
          </div>
        </div>

        <div className="border-b border-rule px-4 py-4">
          <div className="flex items-center justify-between">
            <span className="eyebrow">History</span>
            <Link to={`/dashboard/customers/${current.customer.id}`} className="text-[10.5px] font-semibold text-accent-ink hover:underline">Profile</Link>
          </div>
          {current.recentConversations.length ? (
            <div className="mt-2 space-y-1">
              {current.recentConversations.slice(0, 5).map((item) => (
                <Link key={item.id} to={`/dashboard/inbox/${item.id}`} className="flex items-center gap-2 rounded-md px-1.5 py-1.5 text-[11px] transition-colors hover:bg-surface-2">
                  <span className={cn("size-1.5 rounded-full", statusDotClass(item.status))} />
                  <span className="capitalize text-muted-foreground">{titleCase(item.status)}</span>
                  <span className="ml-auto font-mono text-[9px] text-subtle">{relativeTime(item.createdAt)}</span>
                </Link>
              ))}
            </div>
          ) : (
            <p className="mt-2 text-[11px] text-muted-foreground">No earlier conversations.</p>
          )}
        </div>
        <div className="border-b border-rule px-4 py-4">
          <span className="eyebrow">Conversation</span>
          <dl className="mt-2 space-y-2 text-[11px]">
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">Opened</dt><dd className="font-mono text-[10px] text-ink-2">{dateTime(current.conversation.createdAt)}</dd></div>
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">Channel</dt><dd className="capitalize text-ink-2">{current.conversation.channel}</dd></div>
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">Assigned</dt><dd className="text-right text-ink-2">{assignedName || "Unassigned"}</dd></div>
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">Conversation ID</dt><dd><CopyButton value={current.conversation.id} label="Copy" iconOnly className="border-0 bg-transparent" /></dd></div>
          </dl>
        </div>
        <div className="px-4 py-4">
          <span className="eyebrow">Recent agent actions</span>
          {current.actions.length ? (
            <div className="mt-2 space-y-2">
              {current.actions.slice(0, 5).map((action) => (
                <div key={action.id} className="rounded-md border border-rule bg-surface px-2.5 py-2">
                  <div className="flex items-center gap-2">
                    <span className={cn("size-1.5 rounded-full", statusDotClass(action.status))} />
                    <span className="truncate font-mono text-[9.5px] text-ink-2">{action.toolId}</span>
                    <span className="ml-auto font-mono text-[8.5px] uppercase tracking-[0.06em] text-subtle">{action.status.replaceAll("_", " ")}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-2 text-[11px] text-muted-foreground">No tool calls yet.</p>
          )}
        </div>
      </div>
      <div className="shrink-0 border-t border-rule px-4 py-2.5 font-mono text-[8.5px] uppercase tracking-[0.08em] text-subtle">
        Organization scoped · evidence preserved
      </div>
    </>
  );
}

function slaTone(tone: "ok" | "warn" | "danger" | "muted"): string {
  return tone === "danger" ? "text-[var(--color-danger)]" : tone === "warn" ? "text-[var(--color-warn)]" : tone === "ok" ? "text-[var(--color-success)]" : "text-muted-foreground";
}

function FilterSection({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <section className="py-1">
      <p className="eyebrow px-2 py-1.5">{label}</p>
      <div className="space-y-0.5">{children}</div>
    </section>
  );
}

function FilterChoice({
  label,
  count,
  active,
  dot,
  onClick,
}: {
  value: string;
  label: string;
  count?: number;
  active: boolean;
  dot?: string;
  onClick: () => void;
}) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[11.5px] transition-colors hover:bg-surface-3", active ? "bg-surface-2 text-ink" : "text-muted-foreground")}>
      {dot ? <span className={cn("size-1.5 shrink-0 rounded-full", dot)} /> : null}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {typeof count === "number" ? <span className="font-mono text-[9px] text-subtle">{count}</span> : null}
      {active ? <Check className="size-3.5 shrink-0 text-ink" /> : null}
    </button>
  );
}

function InboxListSkeleton() {
  return (
    <div className="space-y-0">
      {Array.from({ length: 7 }).map((_, index) => (
        <div key={index} className="border-b border-rule px-4 py-3.5">
          <div className="flex gap-3">
            <Skeleton className="size-8 rounded-full" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-3 w-28" />
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-2.5 w-24" />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function ThreadSkeleton() {
  return (
    <div className="space-y-5 p-5">
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-16 w-4/5" />
      <Skeleton className="ml-auto h-20 w-3/5" />
      <Skeleton className="h-16 w-4/5" />
    </div>
  );
}

export function InboxPage() {
  const { session } = useOutletContext<ShellContext>();
  const { id } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const debouncedSearch = useDebouncedValue(search, 280);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [activeIndex, setActiveIndex] = useState(-1);
  const [pendingDecision, setPendingDecision] = useState<{ actionId: string; decision: "approve" | "reject"; toolId: string } | null>(null);
  const [contextOpen, setContextOpen] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const threadScrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [nearBottom, setNearBottom] = useState(true);

  const status = searchParams.get("status") ?? "";
  const assignee = searchParams.get("assignee") ?? "";
  const priority = searchParams.get("priority") ?? "";
  const tag = searchParams.get("tag") ?? "";
  const sla = searchParams.get("sla") ?? "";
  const sort = searchParams.get("sort") ?? "newest";
  const activeFilterCount = [assignee, priority, tag, sla].filter(Boolean).length;

  useEffect(() => {
    if (debouncedSearch !== (searchParams.get("q") ?? "")) {
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        if (debouncedSearch.trim()) next.set("q", debouncedSearch.trim());
        else next.delete("q");
        return next;
      }, { replace: true });
    }
  }, [debouncedSearch, searchParams, setSearchParams]);

  useEffect(() => {
    setSearch(searchParams.get("q") ?? "");
  }, [searchParams]);

  useEffect(() => {
    setSelected(new Set());
    setSelectMode(false);
  }, [status, assignee, priority, tag, sla, sort, debouncedSearch]);

  const params = useMemo(() => {
    const next = new URLSearchParams();
    if (status) next.set("status", status);
    if (debouncedSearch.trim()) next.set("q", debouncedSearch.trim());
    if (assignee) next.set("assignee", assignee);
    if (priority) next.set("priority", priority);
    if (tag) next.set("tag", tag);
    if (sla) next.set("sla", sla);
    if (sort) next.set("sort", sort);
    return next;
  }, [assignee, debouncedSearch, priority, sla, sort, status, tag]);

  const inbox = useQuery({
    queryKey: ["inbox", params.toString()],
    queryFn: () => api.inbox(params),
    refetchInterval: 30_000,
  });
  const conversation = useQuery({
    queryKey: ["conversation", id],
    queryFn: () => api.conversation(id!),
    enabled: Boolean(id),
    refetchInterval: 20_000,
  });

  const conversations = inbox.data?.conversations ?? [];
  const members = inbox.data?.members ?? [];
  const current = conversation.data;
  const assignedName = current?.conversation.assignedName;

  useEffect(() => {
    if (!id) return;
    const index = conversations.findIndex((item) => item.id === id);
    if (index >= 0) setActiveIndex(index);
  }, [conversations, id]);

  useEffect(() => {
    if (!current) return;
    const carry = Boolean(current.conversation.handoff);
    setNearBottom(!carry);
    const timeout = window.setTimeout(() => {
      if (carry) threadScrollRef.current?.scrollTo({ top: 0, behavior: "auto" });
      else bottomRef.current?.scrollIntoView({ block: "end", behavior: "auto" });
    }, 40);
    return () => window.clearTimeout(timeout);
  }, [current?.conversation.id]);

  useEffect(() => {
    if (nearBottom) bottomRef.current?.scrollIntoView({ block: "end", behavior: "smooth" });
  }, [current?.messages.length, nearBottom]);

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["inbox"] });
    void queryClient.invalidateQueries({ queryKey: ["conversation", id] });
    void queryClient.invalidateQueries({ queryKey: ["overview"] });
    void queryClient.invalidateQueries({ queryKey: ["app-session"] });
  }, [id, queryClient]);

  const resolve = useMutation({
    mutationFn: ({ conversationId, action }: { conversationId: string; action: "resolve" | "close" | "reopen" }) => api.resolve(conversationId, action),
    onSuccess: (_, variables) => {
      invalidate();
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      if (variables.action === "resolve" || variables.action === "close") {
        const closed = variables.action === "close";
        toast.success(closed ? "Ticket closed." : "Ticket resolved.", {
          description: closed ? "It is out of the active queue and still searchable." : "The customer sees the conversation as resolved.",
          action: {
            label: "Undo",
            onClick: () => { void api.resolve(variables.conversationId, "reopen").then(() => { invalidate(); toast.success("Ticket reopened."); }).catch(() => toast.error("Could not reopen the ticket.")); },
          },
        });
      } else {
        toast.success("Ticket reopened.");
      }
    },
    onError: (error) => toast.error("The ticket could not change", { description: error instanceof Error ? error.message : undefined }),
  });

  const updateTicket = useMutation({
    mutationFn: (patch: { priority?: string; tags?: string[]; snoozedUntil?: string; clearSnooze?: boolean }) =>
      api.updateTicket(id!, patch),
    onSuccess: (_, patch) => {
      invalidate();
      if (patch.priority) toast.success(`Priority set to ${patch.priority}.`);
      else if (patch.tags) toast.success("Tags updated.");
      else if (patch.clearSnooze) toast.success("Snooze cleared.");
      else toast.success("Ticket snoozed.");
    },
    onError: (error) => toast.error("The ticket could not be updated", { description: error instanceof Error ? error.message : undefined }),
  });

  const assign = useMutation({
    mutationFn: ({ conversationId, userId }: { conversationId: string; userId: string | null }) => api.assign(conversationId, userId),
    onSuccess: (_, variables) => {
      const member = members.find((item) => item.id === variables.userId);
      invalidate();
      toast.success(variables.userId ? `Assigned to ${member?.name ?? "teammate"}.` : "Assignment cleared.");
    },
    onError: (error) => toast.error("The assignment could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  const decide = useMutation({
    mutationFn: ({ actionId, decision }: { actionId: string; decision: "approve" | "reject" }) => api.decideAction(actionId, decision),
    onSuccess: (_, variables) => {
      setPendingDecision(null);
      invalidate();
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      toast.success(variables.decision === "approve" ? "Approved. The agent is running it now." : "Action rejected. Nothing executed.");
    },
    onError: (error) => {
      setPendingDecision(null);
      toast.error("The approval could not be decided", { description: error instanceof Error ? error.message : undefined });
    },
  });

  const bulk = useMutation({
    mutationFn: ({ action, userId, priority, snoozedUntil, clearSnooze }: { action: "resolve" | "close" | "assign" | "priority" | "snooze"; userId?: string | null; priority?: string; snoozedUntil?: string | null; clearSnooze?: boolean }) =>
      api.bulkInbox(Array.from(selected), action, { assignedUserId: userId, priority, snoozedUntil, clearSnooze }),
    onSuccess: (result, variables) => {
      invalidate();
      setSelected(new Set());
      setSelectMode(false);
      if (result.failed.length > 0) toast.error(`${result.failed.length} conversation${result.failed.length === 1 ? "" : "s"} could not be updated`);
      else if (variables.action === "resolve" || variables.action === "close") {
        const closed = variables.action === "close";
        toast.success(`${result.updated.length} ticket${result.updated.length === 1 ? "" : "s"} ${closed ? "closed" : "resolved"}.`, {
          action: {
            label: "Undo",
            onClick: () => { void api.bulkInbox(result.updated, "reopen").then(() => { invalidate(); toast.success("Tickets reopened."); }).catch(() => toast.error("Could not reopen the tickets.")); },
          },
        });
      } else if (variables.action === "priority") {
        toast.success(`Priority set to ${variables.priority} on ${result.updated.length} ticket${result.updated.length === 1 ? "" : "s"}.`);
      } else if (variables.action === "snooze") {
        toast.success(`${result.updated.length} ticket${result.updated.length === 1 ? "" : "s"} ${variables.clearSnooze ? "unsnoozed" : "snoozed"}.`, {
          action: {
            label: "Undo",
            onClick: () => { void api.bulkInbox(result.updated, "snooze", { clearSnooze: true }).then(() => { invalidate(); toast.success("Snoozes cleared."); }).catch(() => toast.error("Could not clear the snoozes.")); },
          },
        });
      } else {
        toast.success(`Assigned ${result.updated.length} conversation${result.updated.length === 1 ? "" : "s"}.`);
      }
    },
    onError: (error) => toast.error("The bulk action failed", { description: error instanceof Error ? error.message : undefined }),
  });

  const setParam = useCallback((key: string, value: string) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (value) next.set(key, value);
      else next.delete(key);
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing = target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable);
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (target?.closest('[role="dialog"], [role="alertdialog"]')) return;
      if (typing) {
        if (event.key === "Escape") (target as HTMLElement).blur();
        return;
      }
      if (event.key === "/") {
        event.preventDefault();
        searchRef.current?.focus();
        return;
      }
      if (event.key === "j" || event.key === "k") {
        event.preventDefault();
        const direction = event.key === "j" ? 1 : -1;
        const start = activeIndex < 0 ? (direction > 0 ? -1 : conversations.length) : activeIndex;
        const nextIndex = Math.max(0, Math.min(conversations.length - 1, start + direction));
        setActiveIndex(nextIndex);
        const next = conversations[nextIndex];
        if (next) navigate(conversationHref(next, searchParams));
        return;
      }
      if (event.key === "Enter" && conversations.length > 0) {
        event.preventDefault();
        const next = conversations[activeIndex] ?? conversations[0];
        if (next) navigate(conversationHref(next, searchParams));
        return;
      }
      if (!current) return;
      if (event.key === "r" || event.key === "n") {
        event.preventDefault();
        const label = event.key === "n" ? "Internal note" : "Reply";
        const tab = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find((item) => item.textContent?.trim() === label);
        tab?.click();
        window.setTimeout(() => document.querySelector<HTMLTextAreaElement>("[data-composer-input]")?.focus(), 20);
        return;
      }
      if (event.key === "e") {
        if (resolve.isPending) return;
        event.preventDefault();
        resolve.mutate({ conversationId: current.conversation.id, action: current.conversation.status === "resolved" || current.conversation.status === "closed" ? "reopen" : "resolve" });
        return;
      }
      if (event.key === "a") {
        event.preventDefault();
        assign.mutate({ conversationId: current.conversation.id, userId: session.user!.id });
        return;
      }
      if (event.key === "c") {
        const button = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find((item) => item.textContent?.includes("Draft with copilot"));
        button?.click();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [activeIndex, assign, conversations, current, navigate, resolve, searchParams, session.user]);

  function toggleSelected(conversationId: string, checked: boolean) {
    setSelected((currentSelection) => {
      const next = new Set(currentSelection);
      if (checked) next.add(conversationId);
      else next.delete(conversationId);
      return next;
    });
  }

  const searchSuffix = searchParams.toString() ? `?${searchParams.toString()}` : "";
  const contextPanel = current ? (
    <ConversationContextPanel
      key={current.conversation.id}
      current={current}
      assignedName={assignedName}
      onPriorityChange={(next) => updateTicket.mutate({ priority: next })}
      onTagsChange={(tags) => updateTicket.mutate({ tags })}
      onSnooze={(until) => updateTicket.mutate(until ? { snoozedUntil: until } : { clearSnooze: true })}
      onClose={() => resolve.mutate({ conversationId: current.conversation.id, action: "close" })}
      ticketUpdating={updateTicket.isPending}
    />
  ) : null;

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] min-h-0 overflow-hidden bg-paper">
      <aside className={cn("min-h-0 w-full shrink-0 flex-col border-r border-rule bg-surface/35", id ? "hidden lg:flex" : "flex", "lg:w-[354px] xl:w-[380px]")}>
        <div className="shrink-0 border-b border-rule bg-paper px-4 pb-3 pt-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2">
                <h1 className="font-display text-[30px] leading-none">Inbox</h1>
                <span className="rounded-full border border-rule bg-surface px-2 py-0.5 font-mono text-[9px] font-semibold text-muted-foreground">{inbox.data?.counts.open ?? 0} open</span>
              </div>
              <p className="mt-1 text-[11.5px] text-muted-foreground">Every conversation, evidence trail, and handoff.</p>
            </div>
            <div className="flex items-center gap-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <button type="button" onClick={() => inbox.refetch()} className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink" aria-label="Refresh inbox">
                    <RefreshCw className={cn("size-3.5", inbox.isFetching && "animate-spin")} />
                  </button>
                </TooltipTrigger>
                <TooltipContent>Refresh</TooltipContent>
              </Tooltip>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button type="button" className={cn("flex h-7 items-center gap-1 rounded-md border px-2 text-[10.5px] font-medium transition-colors", selectMode ? "border-ink bg-ink text-paper" : "border-rule text-muted-foreground hover:bg-surface-3 hover:text-ink")} aria-label="Select conversations">
                    <Check className="size-3" /> {selectMode ? "Selecting" : "Select"}
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => { setSelectMode((value) => !value); setSelected(new Set()); }}>{selectMode ? "Leave selection mode" : "Select conversations"}</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setSelected(new Set(conversations.map((item) => item.id)))} disabled={conversations.length === 0}>Select every visible</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          <div className="relative mt-3">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
            <input
              ref={searchRef}
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search names, emails, messages..."
              className="h-8 w-full rounded-md border border-rule bg-surface pl-8 pr-14 text-[12px] text-ink outline-none placeholder:text-subtle focus:border-rule-strong focus:ring-2 focus:ring-ring/20"
              aria-label="Search conversations"
            />
            <kbd className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2">/</kbd>
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-1 pb-0.5">
            {statusFilters.map((filter) => {
              const count = filter.key === "" ? inbox.data?.counts.total : filter.key === "open" ? inbox.data?.counts.open : filter.key === "resolved" ? inbox.data?.counts.resolved : inbox.data?.counts[filter.key];
              return (
                <button
                  key={filter.key}
                  type="button"
                  onClick={() => setParam("status", filter.key)}
                  className={cn(
                    "flex h-6 shrink-0 items-center gap-1.5 rounded-full border px-2 text-[10.5px] font-medium transition-colors",
                    status === filter.key ? "border-ink bg-ink text-paper" : "border-rule bg-surface text-muted-foreground hover:text-ink",
                  )}
                >
                  {filter.label}
                  {typeof count === "number" && count > 0 ? <span className={cn("font-mono text-[9px]", status === filter.key ? "text-paper/65" : "text-subtle")}>{count}</span> : null}
                </button>
              );
            })}
          </div>

          <div className="mt-2 flex items-center justify-between gap-2">
            <Popover>
              <PopoverTrigger asChild>
                <button type="button" className={cn("flex h-7 items-center gap-1.5 rounded-md border px-2 text-[10px] font-medium transition-colors", activeFilterCount > 0 ? "border-rule-strong bg-surface text-ink" : "border-transparent text-muted-foreground hover:bg-surface-3 hover:text-ink")} aria-label="Filter tickets">
                  <SlidersHorizontal className="size-3" /> Filters
                  {activeFilterCount > 0 ? <span className="flex size-4 items-center justify-center rounded-full bg-ink font-mono text-[8px] text-paper">{activeFilterCount}</span> : null}
                </button>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-72 p-0">
                <div className="flex items-center justify-between border-b border-rule px-3.5 py-2.5">
                  <span className="eyebrow">Ticket filters</span>
                  {activeFilterCount > 0 ? <button type="button" onClick={() => { setParam("assignee", ""); setParam("priority", ""); setParam("tag", ""); }} className="text-[10px] font-semibold text-accent-ink hover:underline">Clear all</button> : null}
                </div>
                <div className="max-h-[min(60vh,28rem)] overflow-y-auto p-2">
                  <FilterSection label="Assignment">
                    {[
                      { value: "", label: "Everyone" },
                      { value: "me", label: "Assigned to me" },
                      { value: "unassigned", label: "Unassigned" },
                      ...members.map((member) => ({ value: member.id, label: member.name + (member.self ? " (you)" : "") })),
                    ].map((option) => <FilterChoice key={option.value} {...option} active={assignee === option.value} onClick={() => setParam("assignee", option.value)} />)}
                  </FilterSection>
                  <FilterSection label="Priority">
                    {[{ value: "", label: "Any priority" }, ...ticketPriorities].map((option) => <FilterChoice key={option.value} {...option} active={priority === option.value} onClick={() => setParam("priority", option.value)} dot={"dot" in option ? option.dot : undefined} />)}
                  </FilterSection>
                  <FilterSection label="SLA">
                    <FilterChoice value="" label="Any SLA state" active={sla === ""} onClick={() => setParam("sla", "")} />
                    <FilterChoice value="breached" label="Breached" count={inbox.data?.counts.slaBreached} active={sla === "breached"} onClick={() => setParam("sla", "breached")} dot="bg-[var(--color-danger)]" />
                  </FilterSection>
                  <FilterSection label="Tag">
                    <FilterChoice value="" label="Any tag" active={tag === ""} onClick={() => setParam("tag", "")} />
                    {(inbox.data?.availableTags ?? []).map((option) => (
                      <FilterChoice key={option.name} value={option.name} label={`#${option.name}`} count={option.count} active={tag === option.name} onClick={() => setParam("tag", option.name)} />
                    ))}
                    {inbox.data?.availableTags.length === 0 ? <p className="px-2 py-1 text-[10.5px] text-subtle">No tags on tickets yet.</p> : null}
                  </FilterSection>
                </div>
              </PopoverContent>
            </Popover>
            <div className="flex items-center gap-2">
              <span className="flex items-center gap-1 font-mono text-[9px] uppercase tracking-[0.07em] text-subtle"><span className="size-1.5 rounded-full bg-[var(--color-success)] live-dot" /> Live</span>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button type="button" className="flex items-center gap-1 rounded px-1 py-0.5 text-[10px] font-medium text-muted-foreground hover:text-ink">
                    <ListFilter className="size-3" /> {sort === "oldest" ? "Oldest" : sort === "priority" ? "Priority" : "Newest"}
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => setParam("sort", "newest")}>Newest first</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setParam("sort", "oldest")}>Oldest first</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setParam("sort", "priority")}>Priority first</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </div>

        {selected.size > 0 ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[var(--color-sidebar)] bg-ink px-3 py-2 text-paper">
            <span className="font-mono text-[10px] uppercase tracking-[0.08em]">{selected.size} selected</span>
            <div className="ml-auto flex items-center gap-1.5">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="text-paper hover:bg-white/10 hover:text-paper"><Users /> Assign <ChevronDown className="size-3" /></Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuItem onClick={() => bulk.mutate({ action: "assign", userId: session.user!.id })}>Assign to me</DropdownMenuItem>
                  <DropdownMenuItem onClick={() => bulk.mutate({ action: "assign", userId: null })}>Unassign</DropdownMenuItem>
                  <DropdownMenuSeparator />
                  {members.map((member) => (
                    <DropdownMenuItem key={member.id} onClick={() => bulk.mutate({ action: "assign", userId: member.id })}>{member.name}</DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
              <Button variant="ghost" size="sm" className="text-paper hover:bg-white/10 hover:text-paper" onClick={() => bulk.mutate({ action: "resolve" })} loading={bulk.isPending}><CheckCircle2 /> Resolve</Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="text-paper hover:bg-white/10 hover:text-paper"><MoreHorizontal /> More</Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger><Tag /> Set priority</DropdownMenuSubTrigger>
                    <DropdownMenuSubContent>
                      {ticketPriorities.map((option) => (
                        <DropdownMenuItem key={option.value} onClick={() => bulk.mutate({ action: "priority", priority: option.value })}>
                          <span className={cn("size-1.5 rounded-full", option.dot)} /> {option.label}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSub>
                    <DropdownMenuSubTrigger><Clock3 /> Snooze</DropdownMenuSubTrigger>
                    <DropdownMenuSubContent>
                      {snoozeChoices().map((choice) => (
                        <DropdownMenuItem key={choice.label} onClick={() => bulk.mutate({ action: "snooze", snoozedUntil: choice.until.toISOString() })}>{choice.label}</DropdownMenuItem>
                      ))}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onClick={() => bulk.mutate({ action: "snooze", clearSnooze: true })}><RotateCcw /> Clear snooze</DropdownMenuItem>
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem destructive onClick={() => bulk.mutate({ action: "close" })}><X /> Close tickets</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <Button variant="ghost" size="icon-sm" className="text-paper hover:bg-white/10 hover:text-paper" onClick={() => { setSelected(new Set()); setSelectMode(false); }} aria-label="Clear selection"><X /></Button>
            </div>
          </div>
        ) : null}

        <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto">
          {inbox.isLoading ? <InboxListSkeleton /> : inbox.isError ? (
            <ErrorState error={inbox.error} onRetry={() => void inbox.refetch()} compact />
          ) : conversations.length === 0 ? (
            <EmptyState
              icon={Inbox}
              title={debouncedSearch || status || assignee || priority || tag || sla ? "No tickets match." : "The inbox is quiet."}
              body={debouncedSearch || status || assignee || priority || tag || sla ? "Try a different search or clear the filters. The desk only hides what you asked it to hide." : "Widget and API conversations will land here as soon as a customer writes in."}
              compact
              actionLabel={debouncedSearch || status || assignee || priority || tag || sla ? "Clear filters" : "Open knowledge"}
              onAction={debouncedSearch || status || assignee || priority || tag || sla ? () => { setSearch(""); setSearchParams(new URLSearchParams(), { replace: true }); } : undefined}
              actionTo={debouncedSearch || status || assignee || priority || tag || sla ? undefined : "/dashboard/knowledge"}
            />
          ) : (
            conversations.map((item) => (
              <ConversationRow
                key={item.id}
                item={item}
                active={item.id === id}
                selectMode={selectMode}
                selected={selected.has(item.id)}
                onSelect={(checked) => toggleSelected(item.id, checked)}
                search={searchParams}
              />
            ))
          )}
        </div>
        <div className="flex shrink-0 items-center justify-between border-t border-rule bg-surface px-3 py-2 font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">
          <span>{inbox.data?.total ?? 0} shown</span>
          <span className="flex items-center gap-1"><kbd>J</kbd><kbd>K</kbd> move <kbd className="ml-1">↵</kbd> open</span>
        </div>
      </aside>

      <section className={cn("min-h-0 min-w-0 flex-1 flex-col bg-paper", id ? "flex" : "hidden lg:flex")}>
        {!id ? (
          <div className="flex h-full flex-col items-center justify-center px-6 text-center">
            <span className="flex size-12 items-center justify-center rounded-full border border-rule bg-surface-2 text-muted-foreground"><MessageSquareText className="size-5" /></span>
            <h2 className="mt-4 font-display text-[28px]">Choose a conversation.</h2>
            <p className="mt-1 max-w-sm text-[12.5px] leading-5 text-muted-foreground">The thread, the evidence behind every AI reply, and the handoff all live in one place.</p>
            <div className="mt-5 flex flex-wrap items-center justify-center gap-1.5 text-[10px] text-muted-foreground">
              <kbd>J</kbd><span>next</span><kbd className="ml-2">K</kbd><span>previous</span><kbd className="ml-2">↵</kbd><span>open</span><kbd className="ml-2">?</kbd><span>shortcuts</span>
            </div>
          </div>
        ) : conversation.isLoading ? (
          <ThreadSkeleton />
        ) : conversation.isError || !current ? (
          <ErrorState error={conversation.error} title="This conversation could not open" onRetry={() => void conversation.refetch()} />
        ) : (
          <>
            <h1 className="sr-only lg:hidden">Conversation with {current.customer.name || current.customer.email}</h1>
            <header className="flex shrink-0 flex-nowrap items-center gap-2 border-b border-rule bg-surface/35 px-2.5 py-2.5 sm:gap-3 sm:px-4">
              <button type="button" onClick={() => navigate(`/dashboard/inbox${searchSuffix}`)} className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-surface-3 hover:text-ink lg:hidden" aria-label="Back to inbox">
                <ArrowLeft className="size-4" />
              </button>
              <Avatar className="size-8 shrink-0">
                <AvatarFallback className={avatarTone(current.customer.name)}>{current.customer.name?.slice(0, 1).toUpperCase() || "?"}</AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2">
                  <h2 className="truncate text-[13.5px] font-semibold text-ink">{current.customer.name || current.customer.email}</h2>
                  <span className="hidden shrink-0 font-mono text-[9px] uppercase tracking-[0.07em] text-subtle 2xl:inline">{titleCase(current.conversation.channel)}</span>
                </div>
                <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-1.5 overflow-hidden text-[10.5px] text-muted-foreground">
                  <span className="shrink-0 font-mono text-[9px] text-ink-2">#{current.conversation.ticketNumber}</span>
                  <StatusBadge status={current.conversation.status} label={titleCase(current.conversation.status)} className="shrink-0" />
                  {current.conversation.priority !== "normal" ? <PriorityBadge priority={current.conversation.priority} compact className="shrink-0" /> : null}
                  {isSnoozed(current.conversation.snoozedUntil) ? <span className="flex shrink-0 items-center gap-1 text-[var(--color-warn)]"><Clock3 className="size-3" /> snoozed</span> : null}
                  <span className="hidden truncate sm:inline">{current.customer.email}</span>
                  <span className="hidden shrink-0 text-subtle sm:inline">/</span>
                  <span className="hidden shrink-0 sm:inline">opened {relativeTime(current.conversation.createdAt)}</span>
                  {assignedName ? <><span className="hidden shrink-0 text-subtle sm:inline">/</span><span className="hidden shrink-0 sm:inline">assigned to {assignedName}</span></> : null}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="sm" className="px-2" aria-label={assignedName ? `Assigned to ${assignedName}. Change assignment` : "Assign conversation"}>
                      <Users /><span className="hidden sm:inline">{assignedName ? "Assigned" : "Assign"}</span><ChevronDown className="hidden size-3 sm:block" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    <DropdownMenuItem onClick={() => assign.mutate({ conversationId: current.conversation.id, userId: session.user!.id })}>Assign to me</DropdownMenuItem>
                    <DropdownMenuItem onClick={() => assign.mutate({ conversationId: current.conversation.id, userId: null })}>Unassign</DropdownMenuItem>
                    <DropdownMenuSeparator />
                    {members.map((member) => (
                      <DropdownMenuItem key={member.id} onClick={() => assign.mutate({ conversationId: current.conversation.id, userId: member.id })}>{member.name}{member.self ? " (you)" : ""}</DropdownMenuItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>
                <Button
                  variant={current.conversation.status === "resolved" || current.conversation.status === "closed" ? "secondary" : "default"}
                  size="sm"
                  className="px-2.5"
                  onClick={() => resolve.mutate({ conversationId: current.conversation.id, action: current.conversation.status === "resolved" || current.conversation.status === "closed" ? "reopen" : "resolve" })}
                  loading={resolve.isPending}
                >
                  {current.conversation.status === "resolved" || current.conversation.status === "closed" ? <RotateCcw /> : <CheckCircle2 />}
                  {current.conversation.status === "resolved" || current.conversation.status === "closed" ? "Reopen" : "Resolve"}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" aria-label="More conversation actions"><MoreHorizontal /></Button></DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-56">
                    <DropdownMenuItem onClick={() => setContextOpen(true)}><Info /> Customer details</DropdownMenuItem>
                    <DropdownMenuItem onClick={() => void copyText(current.customer.email).then(() => toast.success("Email copied.")).catch(() => toast.error("Could not copy the email."))}><Copy /> Copy email</DropdownMenuItem>
                    <DropdownMenuItem asChild><Link to={`/dashboard/customers/${current.customer.id}`}><Users /> Open customer profile</Link></DropdownMenuItem>
                    {current.conversation.status !== "closed" ? (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem destructive onClick={() => resolve.mutate({ conversationId: current.conversation.id, action: "close" })}><X /> Close ticket</DropdownMenuItem>
                      </>
                    ) : null}
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </header>

            <div
              ref={threadScrollRef}
              onScroll={(event) => {
                const element = event.currentTarget;
                setNearBottom(element.scrollHeight - element.scrollTop - element.clientHeight < 140);
              }}
              className="relative min-h-0 flex-1 overflow-y-auto px-3 py-4 sm:px-5"
            >
              <div className="mx-auto flex max-w-[760px] flex-col gap-4">
                {current.conversation.handoff ? <HandoffCard detail={current} /> : null}
                {current.messages.length === 0 ? (
                  <EmptyState icon={MessageSquareText} title="No messages yet." body="This conversation has not received a message. The channel may still be preparing it." compact />
                ) : (
                  current.messages.map((message) => <MessageBubble key={message.id} message={message} customerName={current.customer.name || "Customer"} />)
                )}
                {current.actions.filter((action) => action.status === "pending_approval").map((action) => (
                  <PendingApprovalCard key={action.id} action={action} onDecide={(target, decision) => setPendingDecision({ actionId: target.id, decision, toolId: target.toolId })} />
                ))}
                <div ref={bottomRef} className="h-px" />
              </div>
              {!nearBottom ? (
                <button
                  type="button"
                  onClick={() => { bottomRef.current?.scrollIntoView({ behavior: "smooth" }); setNearBottom(true); }}
                  className="sticky bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-rule bg-surface px-3 py-1.5 text-[10.5px] font-medium text-muted-foreground shadow-[var(--shadow-float)] hover:text-ink"
                >
                  <ArrowDown className="size-3" /> Jump to latest
                </button>
              ) : null}
            </div>

            <Composer
              key={current.conversation.id}
              conversationId={current.conversation.id}
              status={current.conversation.status}
              onSent={() => { setNearBottom(true); window.setTimeout(() => bottomRef.current?.scrollIntoView({ behavior: "smooth" }), 60); }}
              copilotAvailable={Boolean(session.permissions?.canInbox)}
            />
          </>
        )}
      </section>

      {current ? (
        <aside className="hidden min-h-0 w-[306px] shrink-0 flex-col border-l border-rule bg-surface/35 xl:flex">
          {contextPanel}
        </aside>
      ) : null}

      <Sheet open={contextOpen} onOpenChange={setContextOpen}>
        <SheetContent className="w-[min(94vw,30rem)] p-0">
          <SheetTitle className="sr-only">Customer details</SheetTitle>
          {contextPanel}
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={Boolean(pendingDecision)}
        onOpenChange={(open) => { if (!open) setPendingDecision(null); }}
        title={pendingDecision?.decision === "approve" ? "Approve and run this action?" : "Reject this action?"}
        description={
          pendingDecision?.decision === "approve"
            ? "The agent runs the tool once and verifies the result. This decision is recorded in the audit log."
            : "Nothing executes. The agent sees the rejection and continues with a safe alternative."
        }
        confirmLabel={pendingDecision?.decision === "approve" ? "Approve and run" : "Reject action"}
        destructive={pendingDecision?.decision === "reject"}
        loading={decide.isPending}
        onConfirm={() => {
          if (pendingDecision) decide.mutate({ actionId: pendingDecision.actionId, decision: pendingDecision.decision });
        }}
      />
    </div>
  );
}
