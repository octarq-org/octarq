import {
  Badge,
  Button,
  Field,
  Input,
  Modal,
  PageCard,
  PageRow,
  PageRows,
  PageSection,
  PageShell,
  Select,
  Separator,
  TablePagination,
  toast,
} from "@octarq/plugin-sdk";
import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../../api/client";
import type { LLMProviderDTO, LLMProviderInput, ModelPriceDTO } from "../../api/types";
import { filterModelPrices, paginatePrices, PRICE_PAGE_SIZE } from "./priceList";
import { useTranslation } from "../../i18n";

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
  const [busy, setBusy] = useState(false);
  const [models, setModels] = useState<string[]>([]);
  const [modelsBusy, setModelsBusy] = useState(false);
  const [modelsFailed, setModelsFailed] = useState(false);
  const [priceQuery, setPriceQuery] = useState("");
  const [priceProvider, setPriceProvider] = useState("all");
  const [pricePage, setPricePage] = useState(1);
  const [pricePageSize, setPricePageSize] = useState(PRICE_PAGE_SIZE);

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

  const priceProviders = useMemo(() => {
    const seen = new Set<string>();
    for (const row of prices) if (row.providerId) seen.add(row.providerId);
    return [...seen].sort();
  }, [prices]);

  const filteredPrices = useMemo(
    () => filterModelPrices(prices, priceQuery, priceProvider),
    [prices, priceProvider, priceQuery],
  );

  const pricePages = paginatePrices(filteredPrices, pricePage, pricePageSize);
  const visiblePrices = pricePages.rows;

  const priceLine = (row: ModelPriceDTO) => {
    const parts = [
      t("gateway.priceInput", "In {{value}}").replace("{{value}}", String(row.input)),
    ];
    if (row.cachedInput != null) {
      parts.push(t("gateway.priceCached", "Cached {{value}}").replace("{{value}}", String(row.cachedInput)));
    }
    parts.push(t("gateway.priceOutput", "Out {{value}}").replace("{{value}}", String(row.output)));
    parts.push(t("gateway.priceReasoning", "Reason {{value}}").replace("{{value}}", String(row.reasoning)));
    return parts.join(" · ");
  };

  return (
    <PageShell
      title={t("gateway.title", "Model Gateway")}
      description={t("gateway.subtitle", "Connect the models this workspace uses. Add one provider.")}
    >
      {loadFailed && loaded && (
        <p role="alert" className="mt-4 rounded-lg border border-danger-border bg-danger-bg p-3 text-xs text-danger-fg">
          {t("gateway.loadFailed", "The gateway settings did not load. Reload the page.")}
        </p>
      )}

      <PageSection
        title={t("gateway.providersTitle", "Providers")}
        description={t("gateway.providersDesc", "A provider holds the backend, the endpoint and the API key.")}
        action={
          <Button size="sm" onClick={() => openDraft({ ...emptyDraft })}>
            {t("gateway.addProvider", "Add provider")}
          </Button>
        }
      >
        {loaded && providers.length === 0 ? (
          <PageCard>
            <p className="text-sm text-muted-foreground">{t("gateway.noProviders", "No provider yet. Add one to start.")}</p>
          </PageCard>
        ) : providers.length > 0 ? (
          <PageRows>
            {providers.map((p, idx) => (
              <Fragment key={p.id}>
                {idx > 0 && <Separator />}
                <PageRow
                  title={
                    <span className="flex items-center gap-2">
                      <span>{p.name}</span>
                      {p.isDefault && <Badge>{t("gateway.badgeDefault", "Default")}</Badge>}
                    </span>
                  }
                  description={`${p.backend}${p.model ? ` · ${p.model}` : ""}${p.baseUrl ? ` · ${p.baseUrl}` : ""}`}
                  control={
                    <div className="flex flex-wrap items-center gap-1.5">
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
                  }
                />
              </Fragment>
            ))}
          </PageRows>
        ) : null}
      </PageSection>

      <PageSection
        title={t("gateway.pricesTitle", "Model prices")}
        description={t("gateway.pricesDesc", "Prices come from the bundled AICostBudget pricing dataset. Update the application to refresh them.")}
      >
        {loaded && prices.length === 0 ? (
          <PageCard>
            <p className="text-sm text-muted-foreground">{t("gateway.noPrices", "The pricing dataset lists no billable model.")}</p>
          </PageCard>
        ) : prices.length > 0 ? (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <Input
                aria-label={t("gateway.pricesSearch", "Search models")}
                placeholder={t("gateway.pricesSearch", "Search models")}
                value={priceQuery}
                onChange={(e) => {
                  setPriceQuery(e.target.value);
                  setPricePage(1);
                }}
                className="h-9 w-full max-w-xs"
              />
              <Select
                aria-label={t("gateway.pricesProvider", "Provider")}
                value={priceProvider}
                onValueChange={(value) => {
                  setPriceProvider(value);
                  setPricePage(1);
                }}
                options={[
                  { value: "all", label: t("gateway.pricesAllProviders", "All providers") },
                  ...priceProviders.map((id) => ({ value: id, label: id })),
                ]}
                className="h-9 w-44 shrink-0"
              />
              <span className="text-xs text-muted-foreground">
                {t("gateway.pricesShown", "{{shown}} of {{total}} models")
                  .replace("{{shown}}", String(filteredPrices.length))
                  .replace("{{total}}", String(prices.length))}
              </span>
            </div>
            {filteredPrices.length === 0 ? (
              <PageCard>
                <p className="text-sm text-muted-foreground">{t("gateway.noPriceMatches", "No model matches the filter.")}</p>
              </PageCard>
            ) : (
              <PageRows>
                {visiblePrices.map((row, idx) => (
                  <Fragment key={row.model}>
                    {idx > 0 && <Separator />}
                    <PageRow
                      title={
                        <span className="flex items-center gap-2">
                          <span className="font-mono text-xs">{row.model}</span>
                          {row.displayName && row.displayName !== row.model && (
                            <span className="text-xs text-muted-foreground">{row.displayName}</span>
                          )}
                        </span>
                      }
                      description={
                        <span className="flex flex-wrap items-center gap-2">
                          <span>{priceLine(row)}</span>
                          {row.providerId && <Badge>{row.providerId}</Badge>}
                        </span>
                      }
                    />
                  </Fragment>
                ))}
              </PageRows>
            )}
            <TablePagination
              page={pricePages.page}
              pageSize={pricePageSize}
              total={filteredPrices.length}
              pageCount={pricePages.pageCount}
              onPageChange={setPricePage}
              onPageSizeChange={setPricePageSize}
              config={{ hideOnSinglePage: true }}
            />
          </>
        ) : null}
      </PageSection>

      <PageSection
        title={t("gateway.agentsTitle", "Coding agents")}
        description={t("gateway.agentsDesc", "Point a coding agent at this endpoint. The agent reads the model list from the gateway and never holds the provider key.")}
      >
        <PageCard>
          <dl className="grid gap-2.5 text-xs sm:grid-cols-[9rem_1fr]">
            <dt className="text-muted-foreground">{t("gateway.agentsBaseUrl", "Base URL")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`${window.location.origin}/v1`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsKey", "API key")}</dt>
            <dd className="text-foreground">{t("gateway.agentsKeyValue", "A workspace token. Mint one on the tokens page.")}</dd>
            <dt className="text-muted-foreground">{t("gateway.agentsModels", "Model list")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`GET /v1/models`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsChat", "Chat format")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`POST /v1/chat/completions`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsResponses", "Responses format")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`POST /v1/responses`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsMessages", "Anthropic format")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`POST /v1/messages`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsGeminiModels", "Gemini model list")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`GET /v1beta/models`}</code>
            </dd>
            <dt className="text-muted-foreground">{t("gateway.agentsGemini", "Gemini format")}</dt>
            <dd className="min-w-0">
              <code className="block truncate rounded-md border border-border bg-well px-2 py-1 font-mono text-foreground">{`POST /v1beta/models/{model}:generateContent`}</code>
            </dd>
          </dl>
        </PageCard>
      </PageSection>

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
    </PageShell>
  );
}
