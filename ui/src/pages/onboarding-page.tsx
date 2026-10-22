// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  FileText,
  Globe2,
  KeyRound,
  LogOut,
  MessageSquare,
  ShieldCheck,
  Sparkles,
  UploadCloud,
} from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { sessionKey, useSession } from "@/hooks/use-session";
import { cn } from "@/lib/utils";
import { Brand } from "@/components/logo";
import { BootScreen } from "@/components/boot-screen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const providers = [
  { id: "openai", label: "OpenAI", description: "GPT models and embeddings", keyless: false, placeholder: "sk-..." },
  { id: "anthropic", label: "Anthropic", description: "Claude models", keyless: false, placeholder: "sk-ant-..." },
  { id: "google", label: "Google AI", description: "Gemini models and embeddings", keyless: false, placeholder: "AIza..." },
  { id: "ollama", label: "Ollama", description: "Local models, no cloud key", keyless: true, placeholder: "Optional key" },
  { id: "openai-compatible", label: "OpenAI-compatible", description: "Any compatible endpoint", keyless: true, placeholder: "Optional key" },
];

export function OnboardingPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const session = useSession();
  const onboarding = session.data?.onboarding;
  const [step, setStep] = useState(1);
  const [providerId, setProviderId] = useState("openai");
  const [displayName, setDisplayName] = useState("OpenAI");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [providerDone, setProviderDone] = useState(false);
  const [knowledgeDone, setKnowledgeDone] = useState(false);
  const [knowledgeTab, setKnowledgeTab] = useState("url");
  const [sourceUrl, setSourceUrl] = useState("");
  const [sourceName, setSourceName] = useState("");
  const [sourceText, setSourceText] = useState("");
  const [sourceScope, setSourceScope] = useState("public");
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [error, setError] = useState("");
  const [finishing, setFinishing] = useState(false);
  const selectedProvider = providers.find((provider) => provider.id === providerId)!;

  useEffect(() => {
    if (!onboarding) return;
    setProviderDone(onboarding.providerConfigured);
    setKnowledgeDone(onboarding.knowledgeAdded);
    if (onboarding.knowledgeAdded) setStep(3);
    else if (onboarding.providerConfigured) setStep(2);
  }, [onboarding]);

  if (session.isLoading) return <BootScreen />;
  if (!session.data?.authenticated) return <Navigate to="/login?next=/onboarding" replace />;
  if (session.data.onboarding?.complete) return <Navigate to="/dashboard" replace />;

  const saveProvider = useMutation({
    mutationFn: () =>
      api.saveProvider({
        providerId,
        displayName: displayName.trim() || selectedProvider.label,
        apiKey,
        baseUrl,
      }),
    onSuccess: () => {
      setProviderDone(true);
      setStep(2);
      toast.success("Provider connected.", { description: "Next, give the agent something to answer from." });
      void queryClient.invalidateQueries({ queryKey: sessionKey });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "The provider could not be saved."),
  });

  const addSource = useMutation({
    mutationFn: async () => {
      if (knowledgeTab === "file") {
        if (!sourceFile) throw new ApiError("Choose a file to index.");
        const form = new FormData();
        form.append("file", sourceFile);
        form.append("scope", sourceScope);
        return api.createSource(form);
      }
      if (knowledgeTab === "url") {
        if (!sourceUrl.trim()) throw new ApiError("Enter a public page URL.");
        return api.createSource({ kind: "url", url: sourceUrl.trim(), name: sourceName.trim(), scope: sourceScope });
      }
      if (!sourceName.trim()) throw new ApiError("Give the pasted text a name.");
      if (!sourceText.trim()) throw new ApiError("Paste some text to index.");
      return api.createSource({ kind: "text", name: sourceName.trim(), text: sourceText, scope: sourceScope });
    },
    onSuccess: () => {
      setKnowledgeDone(true);
      setStep(3);
      toast.success("Knowledge added.", { description: "Indexing runs in the background. You can open the desk while it finishes." });
      void queryClient.invalidateQueries({ queryKey: sessionKey });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["knowledge"] });
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "The knowledge source could not be added."),
  });

  async function finish() {
    setFinishing(true);
    try {
      await api.completeOnboarding(true);
      await queryClient.invalidateQueries({ queryKey: sessionKey });
      navigate("/dashboard", { replace: true });
    } catch (err) {
      toast.error("Setup could not finish", { description: err instanceof Error ? err.message : undefined });
    } finally {
      setFinishing(false);
    }
  }

  async function skip() {
    setFinishing(true);
    try {
      await api.completeOnboarding(true);
      await queryClient.invalidateQueries({ queryKey: sessionKey });
      navigate("/dashboard", { replace: true });
    } catch {
      toast.error("Could not skip setup. Try again.");
    } finally {
      setFinishing(false);
    }
  }

  async function signOut() {
    await api.logout();
    queryClient.setQueryData(sessionKey, { authenticated: false, setupRequired: false });
    navigate("/login", { replace: true });
  }

  const steps = [
    { number: 1, label: "Model provider", description: "Give the agent a model.", done: providerDone },
    { number: 2, label: "Knowledge", description: "Give it something to answer from.", done: knowledgeDone },
    { number: 3, label: "Open the desk", description: "Let the first conversation in.", done: false },
  ];

  return (
    <div className="min-h-dvh bg-paper">
      <header className="flex items-center justify-between border-b border-rule px-5 py-4 sm:px-8">
        <Brand className="-ml-0.5" />
        <div className="flex items-center gap-3">
          <span className="hidden font-mono text-[9px] uppercase tracking-[0.09em] text-subtle sm:inline">{session.data.org?.name}</span>
          <button type="button" onClick={() => void signOut()} className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-surface-3 hover:text-ink"><LogOut className="size-3.5" /> Sign out</button>
        </div>
      </header>
      <div className="mx-auto grid min-h-[calc(100dvh-4.2rem)] max-w-[1120px] gap-0 lg:grid-cols-[21rem_minmax(0,1fr)]">
        <aside className="border-b border-rule px-5 py-8 sm:px-8 lg:border-b-0 lg:border-r lg:px-8 lg:py-12">
          <p className="eyebrow">First run / {session.data.user?.name?.split(" ")[0]}</p>
          <h1 className="mt-2 font-display text-[38px] leading-[1.02] tracking-[-0.025em]">Open the desk.</h1>
          <p className="mt-2 text-[12.5px] leading-5 text-muted-foreground">Two small decisions before the first customer arrives. You can change both later.</p>
          <ol className="mt-8 space-y-0">
            {steps.map((item, index) => {
              const active = step === item.number;
              return (
                <li key={item.number} className="relative flex gap-3 pb-6 last:pb-0">
                  {index < steps.length - 1 ? <span className={cn("absolute left-[11px] top-6 h-[calc(100%-0.75rem)] w-px", item.done ? "bg-[var(--color-success)]" : "bg-rule")} /> : null}
                  <span className={cn("relative z-10 flex size-6 shrink-0 items-center justify-center rounded-full border font-mono text-[9px] font-semibold", item.done ? "border-[var(--color-success)] bg-[var(--color-success)] text-paper" : active ? "border-ink bg-ink text-paper" : "border-rule bg-paper text-subtle")}>
                    {item.done ? <Check className="size-3" /> : item.number}
                  </span>
                  <span className="pt-0.5">
                    <span className={cn("block text-[12.5px] font-semibold", active || item.done ? "text-ink" : "text-muted-foreground")}>{item.label}</span>
                    <span className="mt-0.5 block text-[10.5px] text-muted-foreground">{item.description}</span>
                  </span>
                </li>
              );
            })}
          </ol>
          <div className="mt-10 rounded-lg border border-rule bg-surface px-4 py-3.5">
            <p className="flex items-center gap-1.5 text-[11px] font-semibold"><ShieldCheck className="size-3.5 text-[var(--color-success)]" /> You can pause.</p>
            <p className="mt-1 text-[10.5px] leading-5 text-muted-foreground">Everything here is under Settings later. Skipping setup pauses AI answering, not the desk.</p>
          </div>
        </aside>

        <main className="px-5 py-8 sm:px-8 lg:px-12 lg:py-12">
          <div className="mx-auto max-w-2xl">
            {error ? <div role="alert" className="mb-4 rounded-md border border-[var(--color-danger-rule)] bg-[var(--color-danger-soft)] px-3 py-2.5 text-[12px] text-[var(--color-danger)]">{error}</div> : null}

            {step === 1 ? (
              <div className="animate-[page-enter_220ms_var(--ease-out)_both]">
                <p className="eyebrow">Step 1 of 3</p>
                <h2 className="mt-1.5 font-display text-[34px] leading-none">Choose a model.</h2>
                <p className="mt-2 max-w-xl text-[12.5px] leading-5 text-muted-foreground">The desk sends customer questions to this provider. Keys are sealed on the server and never returned in full.</p>
                <div className="mt-5 grid gap-2 sm:grid-cols-2">
                  {providers.map((provider) => (
                    <button key={provider.id} type="button" onClick={() => { setProviderId(provider.id); setDisplayName(provider.label); setError(""); }} className={cn("flex items-start gap-3 rounded-lg border p-3.5 text-left transition-colors", providerId === provider.id ? "border-ink bg-surface-2 shadow-[var(--shadow-whisper)]" : "border-rule bg-surface hover:border-rule-strong")}>
                      <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-md border font-display text-[16px]", providerId === provider.id ? "border-ink bg-ink text-paper" : "border-rule bg-surface-2 text-muted-foreground")}>{provider.label.slice(0, 1)}</span>
                      <span><span className="block text-[12px] font-semibold">{provider.label}</span><span className="mt-0.5 block text-[10.5px] leading-4 text-muted-foreground">{provider.description}</span></span>
                      {providerId === provider.id ? <CheckCircle2 className="ml-auto size-4 shrink-0 text-accent" /> : null}
                    </button>
                  ))}
                </div>
                <form onSubmit={(event) => { event.preventDefault(); setError(""); saveProvider.mutate(); }} className="mt-5 space-y-3 rounded-lg border border-rule bg-surface p-4 sm:p-5">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5"><Label htmlFor="provider-display">Display name</Label><Input id="provider-display" value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder={selectedProvider.label} /></div>
                    <div className="space-y-1.5"><Label htmlFor="provider-key">API key {selectedProvider.keyless ? <span className="font-normal text-subtle">optional</span> : null}</Label><Input id="provider-key" type="password" autoComplete="off" value={apiKey} onChange={(event) => setApiKey(event.target.value)} placeholder={selectedProvider.placeholder} required={!selectedProvider.keyless} /></div>
                  </div>
                  <div className="space-y-1.5"><Label htmlFor="provider-base">Base URL <span className="font-normal text-subtle">optional</span></Label><Input id="provider-base" type="url" value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder={providerId === "ollama" ? "http://localhost:11434/v1" : "https://api.openai.com/v1"} /></div>
                  <div className="flex items-center justify-between gap-3 border-t border-rule pt-3">
                    <p className="text-[10.5px] text-muted-foreground">You can test the connection later from Settings.</p>
                    <Button type="submit" loading={saveProvider.isPending}>Connect provider <ArrowRight /></Button>
                  </div>
                </form>
              </div>
            ) : null}

            {step === 2 ? (
              <div className="animate-[page-enter_220ms_var(--ease-out)_both]">
                <p className="eyebrow">Step 2 of 3</p>
                <h2 className="mt-1.5 font-display text-[34px] leading-none">Give it evidence.</h2>
                <p className="mt-2 max-w-xl text-[12.5px] leading-5 text-muted-foreground">The agent answers only from approved material, and cites what it uses. Start with one page or one policy.</p>
                <div className="mt-5 rounded-lg border border-rule bg-surface p-4 sm:p-5">
                  <Tabs value={knowledgeTab} onValueChange={(value) => { setKnowledgeTab(value); setError(""); }}>
                    <TabsList className="w-full">
                      <TabsTrigger value="url" className="flex-1"><Globe2 /> Website</TabsTrigger>
                      <TabsTrigger value="file" className="flex-1"><UploadCloud /> File</TabsTrigger>
                      <TabsTrigger value="text" className="flex-1"><FileText /> Pasted text</TabsTrigger>
                    </TabsList>
                    <TabsContent value="url" className="space-y-3">
                      <div className="space-y-1.5"><Label htmlFor="onboarding-url">Page URL</Label><Input id="onboarding-url" type="url" value={sourceUrl} onChange={(event) => setSourceUrl(event.target.value)} placeholder="https://docs.example.com/help" autoFocus /></div>
                      <div className="space-y-1.5"><Label htmlFor="onboarding-name">Display name <span className="font-normal text-subtle">optional</span></Label><Input id="onboarding-name" value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Help center" /></div>
                    </TabsContent>
                    <TabsContent value="file" className="space-y-3">
                      <div onClick={() => document.getElementById("onboarding-file")?.click()} className="flex min-h-32 cursor-pointer flex-col items-center justify-center rounded-lg border border-dashed border-rule-strong bg-surface-2/45 px-4 text-center hover:border-accent" role="button" tabIndex={0} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") document.getElementById("onboarding-file")?.click(); }}>
                        <UploadCloud className="size-5 text-muted-foreground" />
                        <p className="mt-2 text-[12px] font-semibold">{sourceFile ? sourceFile.name : "Choose a document"}</p>
                        <p className="mt-0.5 text-[10.5px] text-muted-foreground">PDF, DOCX, TXT, Markdown, or HTML.</p>
                        <input id="onboarding-file" type="file" className="sr-only" accept=".pdf,.docx,.txt,.md,.html,.htm" onChange={(event) => setSourceFile(event.target.files?.[0] ?? null)} />
                      </div>
                    </TabsContent>
                    <TabsContent value="text" className="space-y-3">
                      <div className="space-y-1.5"><Label htmlFor="onboarding-text-name">Name</Label><Input id="onboarding-text-name" value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Refund policy" /></div>
                      <div className="space-y-1.5"><Label htmlFor="onboarding-text">Text</Label><Textarea id="onboarding-text" value={sourceText} onChange={(event) => setSourceText(event.target.value)} placeholder="Paste the exact policy or process..." className="min-h-32" /></div>
                    </TabsContent>
                  </Tabs>
                  <div className="mt-3 flex flex-col gap-3 border-t border-rule pt-3 sm:flex-row sm:items-end sm:justify-between">
                    <div className="w-full sm:w-52"><Label>Scope</Label><Select value={sourceScope} onValueChange={setSourceScope}><SelectTrigger className="mt-1.5" aria-label="Knowledge scope"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="public">Public, can be quoted</SelectItem><SelectItem value="internal">Internal, never quoted</SelectItem></SelectContent></Select></div>
                    <Button onClick={() => addSource.mutate()} loading={addSource.isPending}><BookOpen /> Index source</Button>
                  </div>
                </div>
                <button type="button" onClick={() => setStep(3)} className="mt-3 text-[11px] font-medium text-muted-foreground underline-offset-4 hover:text-ink hover:underline">I will add knowledge later</button>
              </div>
            ) : null}

            {step === 3 ? (
              <div className="animate-[page-enter_220ms_var(--ease-out)_both]">
                <p className="eyebrow">Step 3 of 3</p>
                <h2 className="mt-1.5 font-display text-[34px] leading-none">The desk is open.</h2>
                <p className="mt-2 max-w-xl text-[12.5px] leading-5 text-muted-foreground">The agent is ready to answer from the material you approved. Every reply carries its evidence, and risky tools wait for you.</p>
                <div className="mt-5 grid gap-2 sm:grid-cols-2">
                  {[
                    { icon: Sparkles, title: providerDone ? "Model connected" : "No provider yet", body: providerDone ? "The agent has a model to think with." : "AI answering is paused. Connect one later in settings." },
                    { icon: BookOpen, title: knowledgeDone ? "Knowledge added" : "No knowledge yet", body: knowledgeDone ? "Indexing runs in the background." : "The agent will hand off rather than guess." },
                    { icon: MessageSquare, title: "Ready for the first chat", body: "Widget or API conversations land in the inbox." },
                    { icon: KeyRound, title: "Private by default", body: "No sample data, no outbound telemetry." },
                  ].map((item) => (
                    <div key={item.title} className="flex items-start gap-3 rounded-lg border border-rule bg-surface p-3.5">
                      <span className={cn("flex size-7 shrink-0 items-center justify-center rounded-md", item.title.includes("No ") ? "bg-[var(--color-warn-soft)] text-[var(--color-warn)]" : "bg-[var(--color-success-soft)] text-[var(--color-success)]")}><item.icon className="size-3.5" /></span>
                      <span><span className="block text-[12px] font-semibold">{item.title}</span><span className="mt-0.5 block text-[10.5px] leading-4 text-muted-foreground">{item.body}</span></span>
                    </div>
                  ))}
                </div>
                <div className="mt-6 flex flex-wrap items-center gap-2">
                  <Button onClick={() => void finish()} loading={finishing}>Open the desk <ArrowRight /></Button>
                  <Button variant="ghost" onClick={() => void skip()} disabled={finishing}>Skip for now</Button>
                </div>
              </div>
            ) : null}

            <div className="mt-8 flex items-center justify-between border-t border-rule pt-4">
              <button type="button" onClick={() => step > 1 ? setStep(step - 1) : undefined} disabled={step === 1} className="text-[11px] font-medium text-muted-foreground disabled:opacity-40 hover:text-ink">Back</button>
              <button type="button" onClick={() => void skip()} disabled={finishing} className="text-[11px] font-medium text-muted-foreground underline-offset-4 hover:text-ink hover:underline">Finish setup later</button>
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}
