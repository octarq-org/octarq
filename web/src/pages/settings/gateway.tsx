import { Badge, Button, Field, Input, Modal, PageHeader, Panel, Select, toast } from "@octarq/plugin-sdk";
import { useCallback, useEffect, useState } from "react";
import { api } from "../../api/client";
import type { LLMProviderDTO, LLMProviderInput, ModelPriceDTO } from "../../api/types";
import { useTranslation } from "../../i18n";

// The workspace's model backend lives here, not in each feature: one provider
// row and one price row per model. A feature that calls a model reads them
// through the host, so the price a budget is enforced against and the price a
// turn is billed at come from the same row.

type Draft = LLMProviderInput & { id?: number };

const emptyDraft: Draft = { name: "", backend: "", apiKey: "", baseUrl: "", model: "", cheapModel: "" };

export function GatewaySettings() {
  const { t } = useTranslation();
  const [providers, setProviders] = useState<LLMProviderDTO[]>([]);
  const [prices, setPrices] = useState<ModelPriceDTO[]>([]);
  const [backends, setBackends] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [priceDraft, setPriceDraft] = useState<ModelPriceDTO | null>(null);
  const [busy, setBusy] = useState(false);
  // The model names come from the provider, never from this form. `models` is
  // empty until the provider answers, which is what keeps a typed-in name from
  // reaching the store.
  const [models, setModels] = useState<string[]>([]);
  const [modelsBusy, setModelsBusy] = useState(false);
  const [modelsFailed, setModelsFailed] = useState(false);

  const load = useCallback(async () => {
    try {
      const [nextProviders, nextPrices, nextBackends] = await Promise.all([
        api.llmProviders(),
        api.modelPrices(),
        api.llmBackends(),
      ]);
      setProviders(nextProviders);
      setPrices(nextPrices);
      setBackends(nextBackends);
      setLoadFailed(false);
    } catch {
      setLoadFailed(true);
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const loadModels = useCallback(async (next: Draft) => {
    if (!next.backend) return;
    setModelsBusy(true);
    setModelsFailed(false);
    try {
      const rows = await api.discoverModels({
        ...(next.id ? { providerId: next.id } : {}),
        backend: next.backend,
        baseUrl: next.baseUrl?.trim() ?? "",
        ...(next.apiKey?.trim() ? { apiKey: next.apiKey.trim() } : {}),
      });
      setModels(rows.map((row) => row.id));
      setModelsFailed(rows.length === 0);
    } catch {
      setModels([]);
      setModelsFailed(true);
    } finally {
      setModelsBusy(false);
    }
  }, []);

  const openDraft = (next: Draft) => {
    setModels([]);
    setModelsFailed(false);
    setDraft(next);
    if (next.id) void loadModels(next);
  };

  // The stored value stays selectable even when the provider stops listing it,
  // so an edit does not silently drop the model the workspace runs on.
  const modelOptions = (current: string | undefined) => {
    const names = [...models];
    if (current && !names.includes(current)) names.unshift(current);
    return names.map((name) => ({ value: name, label: name }));
  };

  const save = async () => {
    if (!draft) return;
    setBusy(true);
    try {
      const body: LLMProviderInput = {
        name: draft.name.trim(),
        backend: draft.backend,
        baseUrl: draft.baseUrl?.trim() ?? "",
        model: draft.model?.trim() ?? "",
        cheapModel: draft.cheapModel?.trim() ?? "",
        ...(draft.apiKey?.trim() ? { apiKey: draft.apiKey.trim() } : {}),
      };
      if (draft.id) await api.updateLLMProvider(draft.id, body);
      else await api.createLLMProvider(body);
      setDraft(null);
      toast(t("gateway.saved", "Saved."));
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("gateway.saveFailed", "The change did not save."));
    } finally {
      setBusy(false);
    }
  };

  const savePrice = async () => {
    if (!priceDraft) return;
    setBusy(true);
    try {
      const next = await api.setModelPrice({
        model: priceDraft.model.trim(),
        input: Number(priceDraft.input),
        output: Number(priceDraft.output),
        reasoning: Number(priceDraft.reasoning),
      });
      setPrices(next);
      setPriceDraft(null);
      toast(t("gateway.saved", "Saved."));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("gateway.saveFailed", "The change did not save."));
    } finally {
      setBusy(false);
    }
  };

  const removePrice = async (model: string) => {
    try {
      await api.deleteModelPrice(model);
      setPrices((rows) => rows.filter((r) => r.model !== model));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("gateway.deleteFailed", "The delete failed."));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader title={t("gateway.title", "Model Gateway")} description={t("gateway.subtitle", "Connect the models this workspace uses. Add one provider and its prices.")} />

      {loadFailed && loaded && (
        <p role="alert" className="rounded-xl border border-danger-border bg-danger-bg p-3 text-xs text-danger-fg">
          {t("gateway.loadFailed", "The gateway settings did not load. Reload the page.")}
        </p>
      )}

      <Panel
        title={t("gateway.providersTitle", "Providers")}
        description={t("gateway.providersDesc", "A provider holds the backend, the endpoint and the API key.")}
        action={
          <Button size="sm" onClick={() => openDraft({ ...emptyDraft })}>
            {t("gateway.addProvider", "Add provider")}
          </Button>
        }
      >
        {loaded && providers.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("gateway.noProviders", "No provider yet. Add one to start.")}</p>
        )}
        <div className="space-y-2">
          {providers.map((p) => (
            <div key={p.id} className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-3 py-2">
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-2 truncate text-sm font-medium text-foreground">
                  {p.name}
                  {p.isDefault && <Badge>{t("gateway.badgeDefault", "Default")}</Badge>}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {p.backend}
                  {p.model ? ` · ${p.model}` : ""}
                  {p.baseUrl ? ` · ${p.baseUrl}` : ""}
                </p>
              </div>
              {!p.isDefault && (
                <Button size="sm" variant="outline" onClick={() => void api.setDefaultLLMProvider(p.id).then(load)}>
                  {t("gateway.actionDefault", "Make default")}
                </Button>
              )}
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  void api
                    .testLLMProvider(p.id)
                    .then((r) => toast(t("gateway.testOk", "The provider answered with {{model}}.").replace("{{model}}", r.model)))
                    .catch((err) => toast.error(err instanceof Error ? err.message : t("gateway.testFailed", "The test call failed.")))
                }
              >
                {t("gateway.actionTest", "Test")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  openDraft({
                    id: p.id,
                    name: p.name,
                    backend: p.backend,
                    baseUrl: p.baseUrl,
                    model: p.model,
                    cheapModel: p.cheapModel,
                    apiKey: "",
                  })
                }
              >
                {t("gateway.actionEdit", "Edit")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  void api
                    .deleteLLMProvider(p.id)
                    .then(load)
                    .catch((err) => toast.error(err instanceof Error ? err.message : t("gateway.deleteFailed", "The delete failed.")));
                }}
              >
                {t("gateway.actionDelete", "Delete")}
              </Button>
            </div>
          ))}
        </div>
      </Panel>

      <Panel
        title={t("gateway.pricesTitle", "Model prices")}
        description={t("gateway.pricesDesc", "Enter the price of one million tokens in US dollars. A model without a price cannot bill an agent.")}
        action={
          <Button size="sm" onClick={() => setPriceDraft({ model: "", input: 0, output: 0, reasoning: 0, updatedAt: "" })}>
            {t("gateway.addPrice", "Add price")}
          </Button>
        }
      >
        {loaded && prices.length === 0 && <p className="text-sm text-muted-foreground">{t("gateway.noPrices", "No price yet.")}</p>}
        <div className="space-y-2">
          {prices.map((row) => (
            <div key={row.model} className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-3 py-2">
              <p className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">{row.model}</p>
              <p className="text-xs text-muted-foreground">
                {t("gateway.priceSummary", "In {{input}} · Out {{output}} · Reason {{reasoning}}")
                  .replace("{{input}}", String(row.input))
                  .replace("{{output}}", String(row.output))
                  .replace("{{reasoning}}", String(row.reasoning))}
              </p>
              <Button size="sm" variant="outline" onClick={() => setPriceDraft(row)}>
                {t("gateway.actionEdit", "Edit")}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => void removePrice(row.model)}>
                {t("gateway.actionDelete", "Delete")}
              </Button>
            </div>
          ))}
        </div>
      </Panel>

      <Panel
        title={t("gateway.agentsTitle", "Coding agents")}
        description={t("gateway.agentsDesc", "Point a coding agent at this endpoint. The agent reads the model list from the gateway and never holds the provider key.")}
      >
        <dl className="grid gap-2 text-xs sm:grid-cols-[9rem_1fr]">
          <dt className="text-muted-foreground">{t("gateway.agentsBaseUrl", "Base URL")}</dt>
          <dd className="min-w-0">
            <code className="block truncate rounded-lg border border-border bg-muted px-2 py-1 font-mono text-foreground">{`${window.location.origin}/v1`}</code>
          </dd>
          <dt className="text-muted-foreground">{t("gateway.agentsKey", "API key")}</dt>
          <dd className="text-foreground">{t("gateway.agentsKeyValue", "A workspace token. Mint one on the tokens page.")}</dd>
          <dt className="text-muted-foreground">{t("gateway.agentsModels", "Model list")}</dt>
          <dd className="min-w-0">
            <code className="block truncate rounded-lg border border-border bg-muted px-2 py-1 font-mono text-foreground">{`GET /v1/models`}</code>
          </dd>
        </dl>
      </Panel>

      {draft && (
        <Modal title={draft.id ? t("gateway.editProvider", "Edit provider") : t("gateway.addProvider", "Add provider")} onClose={() => setDraft(null)}>
          <Field label={t("gateway.fieldName", "Name")} hint={t("gateway.fieldNameHint", "A name your team recognises. For example: Production Claude.")}>
            <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </Field>
          <Field label={t("gateway.fieldBackend", "Backend")}>
            <Select
              aria-label={t("gateway.fieldBackend", "Backend")}
              value={draft.backend}
              onValueChange={(v) => setDraft({ ...draft, backend: v })}
              options={backends.map((b) => ({ value: b, label: b }))}
              placeholder={t("gateway.fieldBackendPlaceholder", "Choose a backend")}
            />
          </Field>
          <Field
            label={t("gateway.fieldApiKey", "API key")}
            hint={draft.id ? t("gateway.fieldApiKeyKeep", "Leave empty to keep the stored key.") : undefined}
          >
            <Input type="password" autoComplete="off" value={draft.apiKey ?? ""} onChange={(e) => setDraft({ ...draft, apiKey: e.target.value })} />
          </Field>
          <Field label={t("gateway.fieldBaseUrl", "Base URL")} hint={t("gateway.fieldBaseUrlHint", "Leave empty to use the backend default.")}>
            <Input value={draft.baseUrl ?? ""} onChange={(e) => setDraft({ ...draft, baseUrl: e.target.value })} />
          </Field>
          <div className="mb-3 flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">{t("gateway.modelsHint", "Load the models from the provider. Do not type a model name.")}</p>
            <Button
              size="sm"
              variant="outline"
              className="shrink-0 whitespace-nowrap"
              disabled={modelsBusy || !draft.backend}
              onClick={() => void loadModels(draft)}
            >
              {modelsBusy ? t("gateway.modelsLoading", "Loading…") : t("gateway.modelsLoad", "Load models")}
            </Button>
          </div>
          {modelsFailed && (
            <p role="alert" className="rounded-xl border border-danger-border bg-danger-bg p-3 text-xs text-danger-fg">
              {t("gateway.modelsFailed", "The provider did not list its models. Check the backend, the base URL and the API key.")}
            </p>
          )}
          <Field label={t("gateway.fieldModel", "Model")}>
            <Select
              aria-label={t("gateway.fieldModel", "Model")}
              value={draft.model ?? ""}
              onValueChange={(v) => setDraft({ ...draft, model: v })}
              options={modelOptions(draft.model)}
              disabled={models.length === 0}
              placeholder={t("gateway.modelsPlaceholder", "Load the models first")}
            />
          </Field>
          <Field label={t("gateway.fieldCheapModel", "Cheap model")} hint={t("gateway.fieldCheapModelHint", "Used for short jobs. Leave empty to use the main model.")}>
            <Select
              aria-label={t("gateway.fieldCheapModel", "Cheap model")}
              value={draft.cheapModel ?? ""}
              onValueChange={(v) => setDraft({ ...draft, cheapModel: v })}
              options={[{ value: "", label: t("gateway.cheapModelSame", "Same as the main model") }, ...modelOptions(draft.cheapModel)]}
              disabled={models.length === 0}
              placeholder={t("gateway.modelsPlaceholder", "Load the models first")}
            />
          </Field>
          <div className="mt-2 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDraft(null)}>
              {t("gateway.cancel", "Cancel")}
            </Button>
            <Button disabled={busy || !draft.name.trim() || !draft.backend} onClick={() => void save()}>
              {t("gateway.save", "Save")}
            </Button>
          </div>
        </Modal>
      )}

      {priceDraft && (
        <Modal title={t("gateway.addPrice", "Add price")} onClose={() => setPriceDraft(null)}>
          <Field label={t("gateway.fieldPriceModel", "Model")} hint={t("gateway.fieldPriceModelHint", "The model identifier exactly as the provider reports it.")}>
            <Input value={priceDraft.model} onChange={(e) => setPriceDraft({ ...priceDraft, model: e.target.value })} />
          </Field>
          <Field label={t("gateway.fieldPriceInput", "Input price per million tokens")}>
            <Input type="number" min="0" step="0.01" value={priceDraft.input} onChange={(e) => setPriceDraft({ ...priceDraft, input: Number(e.target.value) })} />
          </Field>
          <Field label={t("gateway.fieldPriceOutput", "Output price per million tokens")}>
            <Input type="number" min="0" step="0.01" value={priceDraft.output} onChange={(e) => setPriceDraft({ ...priceDraft, output: Number(e.target.value) })} />
          </Field>
          <Field label={t("gateway.fieldPriceReasoning", "Reasoning price per million tokens")}>
            <Input
              type="number"
              min="0"
              step="0.01"
              value={priceDraft.reasoning}
              onChange={(e) => setPriceDraft({ ...priceDraft, reasoning: Number(e.target.value) })}
            />
          </Field>
          <div className="mt-2 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setPriceDraft(null)}>
              {t("gateway.cancel", "Cancel")}
            </Button>
            <Button disabled={busy || !priceDraft.model.trim()} onClick={() => void savePrice()}>
              {t("gateway.save", "Save")}
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
