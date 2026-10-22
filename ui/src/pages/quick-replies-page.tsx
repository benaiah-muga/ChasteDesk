// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Search, Trash2, MessageSquareText } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { CannedReply } from "@/lib/types";
import { relativeTime } from "@/lib/format";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PageSkeleton } from "@/components/boot-screen";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { ConfirmDialog } from "@/components/confirm-dialog";

export function QuickRepliesPage() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ["canned-replies"], queryFn: api.cannedReplies });
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<CannedReply | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<CannedReply | null>(null);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["canned-replies"] });
  };

  const save = useMutation({
    mutationFn: () =>
      editing
        ? api.updateCannedReply(editing.id, title.trim(), body.trim())
        : api.createCannedReply(title.trim(), body.trim()),
    onSuccess: () => {
      toast.success(editing ? "Quick reply updated." : "Quick reply saved.", {
        description: "It is available from the reply composer.",
      });
      setDialogOpen(false);
      setEditing(null);
      setTitle("");
      setBody("");
      invalidate();
    },
    onError: (error) => toast.error("The quick reply could not be saved", { description: error instanceof Error ? error.message : undefined }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteCannedReply(id),
    onSuccess: () => {
      setDeleteTarget(null);
      toast.success("Quick reply deleted.");
      invalidate();
    },
    onError: (error) => toast.error("The quick reply could not be deleted", { description: error instanceof Error ? error.message : undefined }),
  });

  if (query.isLoading) return <PageSkeleton />;
  if (query.isError || !query.data) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  const replies = query.data.replies.filter((reply) => {
    const term = search.toLowerCase().trim();
    return !term || reply.title.toLowerCase().includes(term) || reply.body.toLowerCase().includes(term);
  });

  function openCreate() {
    setEditing(null);
    setTitle("");
    setBody("");
    setDialogOpen(true);
  }

  function openEdit(reply: CannedReply) {
    setEditing(reply);
    setTitle(reply.title);
    setBody(reply.body);
    setDialogOpen(true);
  }

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Saved replies</>}
        title="Quick replies"
        description="Reusable answers for the questions this desk hears often. Everyone can use them in the composer; edit them here when the policy changes."
        meta={<><span>{query.data.replies.length} saved</span><span className="text-subtle">/</span><span>Shared with the workspace</span></>}
        actions={<Button onClick={openCreate}><Plus /> New quick reply</Button>}
      />

      <div className="relative max-w-md">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
        <input
          type="search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search quick replies..."
          className="h-9 w-full rounded-md border border-rule-strong bg-surface pl-9 pr-3 text-[13px] outline-none placeholder:text-subtle focus:ring-2 focus:ring-ring/20"
          aria-label="Search quick replies"
        />
      </div>

      {replies.length === 0 ? (
        <Card>
          <EmptyState
            icon={MessageSquareText}
            title={search ? "No replies match." : "No quick replies yet."}
            body={search ? "Try a different search." : "Save the answer to a question you write more than twice. The next teammate can send it in two clicks."}
            actionLabel={search ? "Clear search" : "Create the first reply"}
            onAction={() => { if (search) setSearch(""); else openCreate(); }}
          />
        </Card>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {replies.map((reply) => (
            <Card key={reply.id} className="flex flex-col p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <h2 className="truncate text-[13px] font-semibold">{reply.title}</h2>
                  <p className="mt-0.5 font-mono text-[9px] uppercase tracking-[0.07em] text-subtle">updated {relativeTime(reply.updatedAt)}</p>
                </div>
                <div className="flex shrink-0 items-center gap-0.5">
                  <Button variant="ghost" size="icon-sm" onClick={() => openEdit(reply)} aria-label={`Edit ${reply.title}`}><Pencil /></Button>
                  <Button variant="ghost" size="icon-sm" onClick={() => setDeleteTarget(reply)} aria-label={`Delete ${reply.title}`}><Trash2 /></Button>
                </div>
              </div>
              <p className="mt-3 line-clamp-4 whitespace-pre-wrap text-[12px] leading-5 text-muted-foreground">{reply.body}</p>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? "Edit quick reply" : "New quick reply"}</DialogTitle>
            <DialogDescription>A short name for the library and the exact text inserted into the composer.</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!title.trim() || !body.trim()) return;
              save.mutate();
            }}
            className="space-y-4"
          >
            <div className="space-y-1.5">
              <Label htmlFor="reply-title">Name</Label>
              <Input id="reply-title" value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Billing email change" maxLength={80} autoFocus required />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="reply-body">Reply</Label>
              <Textarea id="reply-body" value={body} onChange={(event) => setBody(event.target.value)} placeholder="Open Settings, then Billing, and update the invoice email." maxLength={4000} className="min-h-36" required />
              <p className="text-right font-mono text-[9px] text-subtle">{body.length}/4000</p>
            </div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" loading={save.isPending} disabled={!title.trim() || !body.trim()}>{editing ? "Save changes" : "Save reply"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}
        title="Delete this quick reply?"
        description={`“${deleteTarget?.title ?? ""}” will disappear from the composer for everyone in this workspace.`}
        confirmLabel="Delete reply"
        destructive
        loading={remove.isPending}
        onConfirm={() => { if (deleteTarget) remove.mutate(deleteTarget.id); }}
      />
    </div>
  );
}
