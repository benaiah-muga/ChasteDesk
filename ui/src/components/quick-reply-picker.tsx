// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { ArrowUpRight, MessageSquareText, Search } from "lucide-react";
import { api } from "@/lib/api";
import { truncate } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

export function QuickReplyPicker({ onInsert }: { onInsert: (body: string) => void }) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const query = useQuery({
    queryKey: ["canned-replies"],
    queryFn: api.cannedReplies,
    enabled: open,
    staleTime: 30_000,
  });
  const replies = (query.data?.replies ?? []).filter((reply) => {
    const term = search.toLowerCase().trim();
    return !term || reply.title.toLowerCase().includes(term) || reply.body.toLowerCase().includes(term);
  });

  return (
    <Popover open={open} onOpenChange={(next) => { setOpen(next); if (!next) setSearch(""); }}>
      <PopoverTrigger asChild>
        <Button type="button" variant="ghost" size="sm"><MessageSquareText /> Quick replies</Button>
      </PopoverTrigger>
      <PopoverContent align="start" side="top" className="w-[22rem] p-0">
        <div className="border-b border-rule p-3">
          <div className="flex items-center justify-between">
            <span className="eyebrow">Quick replies</span>
            <Link to="/dashboard/replies" onClick={() => setOpen(false)} className="flex items-center gap-1 text-[10.5px] font-semibold text-accent-ink hover:underline">
              Manage <ArrowUpRight className="size-3" />
            </Link>
          </div>
          <div className="relative mt-2">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search saved replies..."
              className="h-8 w-full rounded-md border border-rule bg-surface-2 pl-8 pr-2 text-[11.5px] outline-none placeholder:text-subtle focus:ring-2 focus:ring-ring/20"
              aria-label="Search quick replies"
              autoFocus
            />
          </div>
        </div>
        <div className="max-h-64 overflow-y-auto p-1.5">
          {query.isLoading ? <p className="px-2.5 py-4 text-[11px] text-muted-foreground">Loading saved replies...</p> : null}
          {!query.isLoading && replies.length === 0 ? (
            <div className="px-2.5 py-4 text-center">
              <p className="text-[11.5px] font-medium">{search ? "No replies match." : "No saved replies yet."}</p>
              <p className="mt-1 text-[10.5px] leading-4 text-muted-foreground">Save a reply once, then reuse it from every composer.</p>
              <Button variant="secondary" size="sm" className="mt-3" asChild>
                <Link to="/dashboard/replies" onClick={() => setOpen(false)}><MessageSquareText /> Create a quick reply</Link>
              </Button>
            </div>
          ) : null}
          {replies.map((reply) => (
            <button
              key={reply.id}
              type="button"
              onClick={() => { onInsert(reply.body); setOpen(false); }}
              className="w-full rounded-md px-2.5 py-2 text-left transition-colors hover:bg-surface-3"
            >
              <span className="block text-[11.5px] font-semibold text-ink">{reply.title}</span>
              <span className="mt-0.5 block text-[10.5px] leading-4 text-muted-foreground">{truncate(reply.body, 150)}</span>
            </button>
          ))}
        </div>
        <div className="border-t border-rule px-3 py-2 text-[9.5px] text-subtle">Inserted as customer-facing reply text. Review before sending.</div>
      </PopoverContent>
    </Popover>
  );
}
