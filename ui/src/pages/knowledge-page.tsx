// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import {
  AlertTriangle,
  BookOpen,
  Clock3,
  FileText,
  Globe2,
  Loader2,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  UploadCloud,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { KnowledgeSource } from "@/lib/types";
import { cn } from "@/lib/utils";
import { dateTime, relativeTime, titleCase } from "@/lib/format";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { ErrorState } from "@/components/error-state";
import { EmptyState } from "@/components/empty-state";
import { StatusBadge } from "@/components/status-badge";
import { CopyButton } from "@/components/copy-button";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { PageSkeleton } from "@/components/boot-screen";

function sourceIcon(type: string) {
  if (type === "url") return Globe2;
  if (type === "file") return FileText;
  return BookOpen;
}

function sourceTypeLabel(type: string): string {
  if (type === "url") return "Website";
  if (type === "file") return "File";
  return "Pasted text";
}

function AddSourceDialog({
  open,
  onOpenChange,
  canWrite,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canWrite: boolean;
}) {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState("url");
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [text, setText] = useState("");
  const [scope, setScope] = useState("public");
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setTab("url");
    setUrl("");
    setName("");
    setText("");
    setFile(null);
    setError("");
    setScope("public");
  }, [open]);

  const create = useMutation({
    mutationFn: async () => {
      if (tab === "file") {
        if (!file) throw new ApiError("Choose a file to index.");
        const form = new FormData();
        form.append("file", file);
        form.append("scope", scope);
        return api.createSource(form);
      }
      if (tab === "url") {
        if (!url.trim()) throw new ApiError("Enter a public page URL.");
        return api.createSource({ kind: "url", url: url.trim(), name: name.trim(), scope });
      }
      if (!name.trim()) throw new ApiError("Give the pasted text a name.");
      if (!text.trim()) throw new ApiError("Paste the text to index.");
      return api.createSource({ kind: "text", name: name.trim(), text, scope });
    },
    onSuccess: () => {
      toast.success("Source added. Indexing started.", { description: "The agent can use it as soon as the status turns ready." });
      void queryClient.invalidateQueries({ queryKey: ["knowledge"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      onOpenChange(false);
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "The source could not be added."),
  });

  if (!canWrite) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Add a knowledge source</DialogTitle>
          <DialogDescription>Index a page, a document, or text the agent is allowed to quote. Internal sources are available to the agent but never quoted to customers.</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setError("");
            create.mutate();
          }}
          className="space-y-4"
        >
          {error ? <div role="alert" className="rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] leading-5 text-[var(--color-danger)]">{error}</div> : null}
          <Tabs value={tab} onValueChange={(value) => { setTab(value); setError(""); }}>
            <TabsList className="w-full">
              <TabsTrigger value="url" className="flex-1"><Globe2 /> Website</TabsTrigger>
              <TabsTrigger value="file" className="flex-1"><UploadCloud /> File</TabsTrigger>
              <TabsTrigger value="text" className="flex-1"><FileText /> Pasted text</TabsTrigger>
            </TabsList>
            <TabsContent value="url" className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="source-url">Page URL</Label>
                <Input id="source-url" type="url" value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://docs.example.com/help/billing" autoFocus />
                <p className="text-[10.5px] text-muted-foreground">One page per source. Redirects are revalidated and private networks are blocked.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="source-name-url">Display name <span className="font-normal text-subtle">optional</span></Label>
                <Input id="source-name-url" value={name} onChange={(event) => setName(event.target.value)} placeholder="Billing help center" maxLength={200} />
              </div>
            </TabsContent>
            <TabsContent value="file" className="space-y-3">
              <div
                onDragOver={(event) => { event.preventDefault(); setDragging(true); }}
                onDragLeave={() => setDragging(false)}
                onDrop={(event) => {
                  event.preventDefault();
                  setDragging(false);
                  const dropped = event.dataTransfer.files[0];
                  if (dropped) setFile(dropped);
                }}
                className={cn(
                  "flex min-h-36 cursor-pointer flex-col items-center justify-center rounded-lg border border-dashed px-4 py-6 text-center transition-colors",
                  dragging ? "border-accent bg-[var(--color-accent-soft)]/40" : "border-rule-strong bg-surface-2/45 hover:border-rule-strong",
                )}
                onClick={() => inputRef.current?.click()}
                role="button"
                tabIndex={0}
                onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") inputRef.current?.click(); }}
              >
                <input
                  ref={inputRef}
                  type="file"
                  accept=".pdf,.docx,.txt,.md,.html,.htm"
                  className="sr-only"
                  onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                />
                <UploadCloud className="size-6 text-muted-foreground" />
                {file ? (
                  <>
                    <p className="mt-2 text-[12.5px] font-semibold">{file.name}</p>
                    <p className="mt-0.5 text-[10.5px] text-muted-foreground">{(file.size / 1024).toFixed(1)} KB · ready to upload</p>
                  </>
                ) : (
                  <>
                    <p className="mt-2 text-[12.5px] font-semibold">Drop a document here, or choose a file</p>
                    <p className="mt-0.5 text-[10.5px] text-muted-foreground">PDF, DOCX, TXT, Markdown, or HTML. One file per source.</p>
                  </>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="source-name-file">Display name <span className="font-normal text-subtle">optional</span></Label>
                <Input id="source-name-file" value={name} onChange={(event) => setName(event.target.value)} placeholder="Uses the file name if left blank" maxLength={200} />
              </div>
            </TabsContent>
            <TabsContent value="text" className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="source-name-text">Name</Label>
                <Input id="source-name-text" value={name} onChange={(event) => setName(event.target.value)} placeholder="Refund policy, October 2026" maxLength={200} autoFocus />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="source-text">Text</Label>
                <Textarea id="source-text" value={text} onChange={(event) => setText(event.target.value)} placeholder="Paste the exact policy, process, or note the agent may use..." className="min-h-40" maxLength={500_000} />
                <p className="text-right font-mono text-[9px] text-subtle">{text.length.toLocaleString()} / 500,000</p>
              </div>
            </TabsContent>
          </Tabs>
          <div className="flex flex-col gap-2 border-t border-rule pt-4 sm:flex-row sm:items-end sm:justify-between">
            <div className="w-full sm:w-56">
              <Label>Scope</Label>
              <Select value={scope} onValueChange={setScope}>
                <SelectTrigger className="mt-1.5" aria-label="Source scope"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="public">Public, can be quoted</SelectItem>
                  <SelectItem value="internal">Internal, never quoted</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>Cancel</Button>
              <Button type="submit" loading={create.isPending}><Plus /> Add source</Button>
            </DialogFooter>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function SourceSheet({
  source,
  open,
  onOpenChange,
  canWrite,
  onDelete,
  onReindex,
  onScope,
}: {
  source: KnowledgeSource | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canWrite: boolean;
  onDelete: (source: KnowledgeSource) => void;
  onReindex: (source: KnowledgeSource) => void;
  onScope: (source: KnowledgeSource, scope: string) => void;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(94vw,30rem)]">
        {source ? (
          <>
            <div className="border-b border-rule px-5 py-5 pr-12">
              <div className="flex items-start gap-3">
                <span className="flex size-9 items-center justify-center rounded-md border border-rule bg-surface-2 text-muted-foreground">
                  {(() => { const Icon = sourceIcon(source.type); return <Icon className="size-4" />; })()}
                </span>
                <div className="min-w-0">
                  <SheetTitle className="truncate text-[17px] font-semibold">{source.name}</SheetTitle>
                  <div className="mt-1 flex flex-wrap items-center gap-2">
                    <StatusBadge status={source.status} label={titleCase(source.status)} />
                    <Badge variant="outline">{sourceTypeLabel(source.type)}</Badge>
                    <Badge variant={source.scope === "internal" ? "internal" : "outline"}>{source.scope}</Badge>
                  </div>
                </div>
              </div>
            </div>
            <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-5">
              {source.error ? (
                <div className="rounded-lg border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] p-3.5">
                  <p className="flex items-center gap-2 text-[12px] font-semibold text-[var(--color-danger)]"><AlertTriangle className="size-3.5" /> Indexing failed</p>
                  <p className="mt-1.5 text-[11.5px] leading-5 text-[var(--color-danger)]">{source.error}</p>
                </div>
              ) : null}
              <dl className="grid grid-cols-2 gap-3">
                {[
                  ["Version", `v${source.version}`],
                  ["Chunks", source.chunks.toLocaleString()],
                  ["Vector", source.vector ? "Indexed" : "Text search"],
                  ["Embedding model", source.embeddingModel ?? "Not set"],
                  ["Created", dateTime(source.createdAt)],
                  ["Last updated", dateTime(source.updatedAt)],
                ].map(([label, value]) => (
                  <div key={label} className="rounded-md border border-rule bg-surface-2/50 p-2.5">
                    <dt className="font-mono text-[8.5px] uppercase tracking-[0.07em] text-subtle">{label}</dt>
                    <dd className="mt-1 truncate text-[11.5px] font-medium text-ink-2">{value}</dd>
                  </div>
                ))}
              </dl>
              <div>
                <p className="eyebrow">Citation behavior</p>
                <p className="mt-1.5 text-[12px] leading-5 text-muted-foreground">
                  {source.scope === "public"
                    ? "Customers can see this source in citations. The AI may quote it directly."
                    : "The AI may use this source internally, but it will never quote it to a customer."}
                </p>
              </div>
              <div>
                <p className="eyebrow">Source ID</p>
                <div className="mt-1.5 flex items-center gap-2 rounded-md border border-rule bg-surface-2/50 px-2.5 py-2">
                  <code className="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground">{source.id}</code>
                  <CopyButton value={source.id} iconOnly className="border-0 bg-transparent" />
                </div>
              </div>
              {canWrite ? (
                <div className="space-y-2 border-t border-rule pt-4">
                  <Button variant="secondary" className="w-full justify-start" onClick={() => onReindex(source)} disabled={source.status === "indexing" || source.status === "pending" || source.type === "text"}>
                    <RefreshCw /> {source.type === "text" ? "Pasted text cannot be reindexed" : "Reindex this source"}
                  </Button>
                  <Button variant="secondary" className="w-full justify-start" onClick={() => onScope(source, source.scope === "public" ? "internal" : "public")}>
                    <BookOpen /> Make {source.scope === "public" ? "internal" : "public"}
                  </Button>
                  <Button variant="danger" className="w-full justify-start" onClick={() => onDelete(source)}><Trash2 /> Delete source</Button>
                </div>
              ) : null}
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

export function KnowledgePage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [scopeFilter, setScopeFilter] = useState("all");
  const [addOpen, setAddOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<KnowledgeSource | null>(null);
  const [reindexTarget, setReindexTarget] = useState<KnowledgeSource | null>(null);

  useEffect(() => {
    if (searchParams.get("add") === "1") {
      setAddOpen(true);
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        next.delete("add");
        return next;
      }, { replace: true });
    }
    const status = searchParams.get("status");
    if (status) setStatusFilter(status);
  }, [searchParams, setSearchParams]);

  const query = useQuery({
    queryKey: ["knowledge"],
    queryFn: api.knowledge,
    refetchInterval: (current) => {
      const sources = current.state.data?.sources ?? [];
      return sources.some((source) => ["pending", "fetching", "indexing"].includes(source.status)) ? 5_000 : false;
    },
  });

  const updateScope = useMutation({
    mutationFn: ({ id, scope }: { id: string; scope: string }) => api.updateSource(id, { scope }),
    onSuccess: () => {
      toast.success("Source scope updated.");
      void queryClient.invalidateQueries({ queryKey: ["knowledge"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (error) => toast.error("The scope could not change", { description: error instanceof Error ? error.message : undefined }),
  });

  const reindex = useMutation({
    mutationFn: (id: string) => api.reindexSource(id),
    onSuccess: () => {
      setReindexTarget(null);
      toast.success("Reindex queued.", { description: "The source will update in place when the worker finishes." });
      void queryClient.invalidateQueries({ queryKey: ["knowledge"] });
    },
    onError: (error) => toast.error("The source could not be reindexed", { description: error instanceof Error ? error.message : undefined }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteSource(id),
    onSuccess: () => {
      setDeleteTarget(null);
      setSelectedId(null);
      toast.success("Source deleted.", { description: "The agent will no longer retrieve this material." });
      void queryClient.invalidateQueries({ queryKey: ["knowledge"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["app-session"] });
    },
    onError: (error) => toast.error("The source could not be deleted", { description: error instanceof Error ? error.message : undefined }),
  });

  const sources = query.data?.sources ?? [];
  const filtered = useMemo(
    () =>
      sources.filter((source) => {
        const matchesSearch = !search.trim() || source.name.toLowerCase().includes(search.toLowerCase()) || source.type.includes(search.toLowerCase());
        const matchesStatus = statusFilter === "all" || (statusFilter === "processing" ? ["pending", "fetching", "indexing"].includes(source.status) : source.status === statusFilter);
        const matchesScope = scopeFilter === "all" || source.scope === scopeFilter;
        return matchesSearch && matchesStatus && matchesScope;
      }),
    [scopeFilter, search, sources, statusFilter],
  );
  const selected = sources.find((source) => source.id === selectedId) ?? null;

  if (query.isLoading) return <PageSkeleton />;
  if (query.isError || !query.data) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />;
  const counts = query.data.counts;
  const canWrite = query.data.canWrite;

  return (
    <div className="page-enter space-y-6">
      <PageHeader
        eyebrow={<>Desk / Knowledge</>}
        title="Knowledge"
        description="What the desk is allowed to know. Public sources can answer customers; internal-only sources are available to the agent but never quoted."
        meta={
          <>
            <span className="flex items-center gap-1.5"><span className={cn("size-1.5 rounded-full", counts.failed > 0 ? "bg-[var(--color-danger)]" : "bg-[var(--color-success)]")} />{counts.ready} of {counts.total} ready</span>
            <span className="text-subtle">/</span>
            <span>{counts.chunks.toLocaleString()} passages indexed</span>
            <span className="text-subtle">/</span>
            <span>{counts.vectorSearch ? "Vector search" : "Full-text search"}</span>
          </>
        }
        actions={canWrite ? <Button onClick={() => setAddOpen(true)}><Plus /> Add source</Button> : undefined}
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[
          { label: "Sources", value: counts.total, note: `${counts.ready} ready`, icon: BookOpen },
          { label: "Processing", value: counts.processing, note: counts.processing ? "Indexing now" : "Nothing queued", icon: Loader2 },
          { label: "Needs attention", value: counts.failed, note: counts.failed ? "Inspect and retry" : "No failures", icon: AlertTriangle },
          { label: "Retrieval", value: counts.vectorSearch ? "Vector" : "Text", note: counts.vectorSearch ? "Embeddings available" : "Set EMBEDDING_* on the worker", icon: Search },
        ].map((tile) => (
          <Card key={tile.label} className="px-4 py-3.5">
            <div className="flex items-start justify-between gap-2">
              <div>
                <p className="eyebrow">{tile.label}</p>
                <p className="mt-1.5 font-display text-[30px] leading-none">{tile.value}</p>
              </div>
              <tile.icon className={cn("size-4 text-muted-foreground", tile.label === "Processing" && counts.processing > 0 && "animate-spin")} />
            </div>
            <p className="mt-1 text-[10.5px] text-muted-foreground">{tile.note}</p>
          </Card>
        ))}
      </div>

      <div className="flex flex-col gap-3 border-b border-rule pb-4 md:flex-row md:items-center md:justify-between">
        <div className="relative max-w-md flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
          <input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search sources..." className="h-9 w-full rounded-md border border-rule-strong bg-surface pl-9 pr-9 text-[13px] outline-none placeholder:text-subtle focus:ring-2 focus:ring-ring/20" aria-label="Search knowledge sources" />
          {search ? <button type="button" onClick={() => setSearch("")} className="absolute right-2 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded text-muted-foreground hover:bg-surface-3 hover:text-ink" aria-label="Clear search"><X className="size-3.5" /></button> : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex h-8 items-center gap-0.5 rounded-md border border-rule bg-surface-2 p-0.5">
            {[
              { key: "all", label: "All" },
              { key: "ready", label: "Ready" },
              { key: "processing", label: "Processing" },
              { key: "failed", label: "Failed" },
            ].map((item) => (
              <button key={item.key} type="button" onClick={() => setStatusFilter(item.key)} className={cn("h-7 rounded-sm px-2.5 text-[11.5px] font-medium transition-colors", statusFilter === item.key ? "bg-surface text-ink shadow-[var(--shadow-whisper)]" : "text-muted-foreground hover:text-ink")}>{item.label}</button>
            ))}
          </div>
          <Select value={scopeFilter} onValueChange={setScopeFilter}>
            <SelectTrigger className="h-8 w-36 text-[11.5px]" aria-label="Filter by scope"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Every scope</SelectItem>
              <SelectItem value="public">Public</SelectItem>
              <SelectItem value="internal">Internal</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="ghost" size="icon-sm" onClick={() => void query.refetch()} aria-label="Refresh sources"><RefreshCw className={cn("size-3.5", query.isFetching && "animate-spin")} /></Button>
        </div>
      </div>

      {filtered.length === 0 ? (
        <Card>
          <EmptyState
            icon={BookOpen}
            title={sources.length === 0 ? "Give the desk something to stand on." : "No sources match."}
            body={sources.length === 0 ? "Add a help page, a policy document, or a pasted process. The agent will cite what it uses and escalate when the evidence is thin." : "Try a different search or filter. Nothing has been deleted."}
            actionLabel={sources.length === 0 ? "Add the first source" : "Clear filters"}
            onAction={() => { if (sources.length === 0) setAddOpen(true); else { setSearch(""); setStatusFilter("all"); setScopeFilter("all"); } }}
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="border-b border-rule bg-surface-2/50 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
            {filtered.length} {filtered.length === 1 ? "source" : "sources"} · reindexing replaces the previous version atomically
          </div>
          <div className="divide-y divide-rule">
            {filtered.map((source) => {
              const Icon = sourceIcon(source.type);
              return (
                <div key={source.id} className="group relative flex items-start gap-3 px-4 py-3.5 transition-colors hover:bg-surface-2 sm:px-5">
                  <button type="button" onClick={() => setSelectedId(source.id)} className="absolute inset-0 z-0" aria-label={`Open ${source.name}`} />
                  <span className="relative z-10 mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border border-rule bg-surface-2 text-muted-foreground pointer-events-none"><Icon className="size-4" /></span>
                  <div className="relative z-10 min-w-0 flex-1 pointer-events-none">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-[12.5px] font-semibold text-ink">{source.name}</span>
                      <StatusBadge status={source.status} label={titleCase(source.status)} />
                      <Badge variant={source.scope === "internal" ? "internal" : "outline"}>{source.scope}</Badge>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-[10.5px] text-muted-foreground">
                      <span>{sourceTypeLabel(source.type)}</span>
                      <span className="text-subtle">/</span>
                      <span>v{source.version}</span>
                      <span className="text-subtle">/</span>
                      <span>{source.chunks} {source.chunks === 1 ? "chunk" : "chunks"}</span>
                      <span className="text-subtle">/</span>
                      <span>updated {relativeTime(source.updatedAt)}</span>
                    </div>
                    {source.error ? <p className="mt-1.5 line-clamp-1 text-[11px] text-[var(--color-danger)]">{source.error}</p> : null}
                  </div>
                  <div className="relative z-10 flex shrink-0 items-center gap-1">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" aria-label={`Actions for ${source.name}`}><MoreHorizontal /></Button></DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuItem onClick={() => setSelectedId(source.id)}>Details</DropdownMenuItem>
                        {canWrite ? (
                          <>
                            <DropdownMenuItem onClick={() => reindex.mutate(source.id)} disabled={source.status === "indexing" || source.status === "pending" || source.type === "text"}><RefreshCw /> {source.type === "text" ? "Reindex not available" : "Reindex"}</DropdownMenuItem>
                            <DropdownMenuItem onClick={() => updateScope.mutate({ id: source.id, scope: source.scope === "public" ? "internal" : "public" })}><BookOpen /> Make {source.scope === "public" ? "internal" : "public"}</DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem destructive onClick={() => setDeleteTarget(source)}><Trash2 /> Delete</DropdownMenuItem>
                          </>
                        ) : null}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              );
            })}
          </div>
          <div className="flex items-center justify-between border-t border-rule bg-surface-2/40 px-5 py-2.5 font-mono text-[9px] uppercase tracking-[0.08em] text-subtle">
            <span className="flex items-center gap-1"><Clock3 className="size-3" /> {counts.processing > 0 ? "Indexing in progress" : "All changes settled"}</span>
            <span>{counts.chunks.toLocaleString()} passages</span>
          </div>
        </Card>
      )}

      <AddSourceDialog open={addOpen} onOpenChange={setAddOpen} canWrite={canWrite} />
      <SourceSheet
        source={selected}
        open={Boolean(selected)}
        onOpenChange={(open) => { if (!open) setSelectedId(null); }}
        canWrite={canWrite}
        onDelete={(source) => { setSelectedId(null); setDeleteTarget(source); }}
        onReindex={(source) => setReindexTarget(source)}
        onScope={(source, scope) => updateScope.mutate({ id: source.id, scope })}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}
        title="Delete this knowledge source?"
        description={`“${deleteTarget?.name ?? ""}” and all of its indexed passages will be removed. The agent will stop using it immediately. This cannot be undone.`}
        confirmLabel="Delete source"
        destructive
        loading={remove.isPending}
        onConfirm={() => { if (deleteTarget) remove.mutate(deleteTarget.id); }}
      />
      <ConfirmDialog
        open={Boolean(reindexTarget)}
        onOpenChange={(open) => { if (!open) setReindexTarget(null); }}
        title="Reindex this source?"
        description="The current version stays available until the replacement finishes. Previous passages are replaced atomically."
        confirmLabel="Queue reindex"
        loading={reindex.isPending}
        onConfirm={() => { if (reindexTarget) reindex.mutate(reindexTarget.id); }}
      />
    </div>
  );
}
