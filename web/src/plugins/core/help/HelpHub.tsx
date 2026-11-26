import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, ArrowRight, BookOpen, Loader2 } from "lucide-react";
import { Button, PageCard, PageEmpty, PageSection, PageShell } from "@octarq/plugin-sdk";
import { api } from "../../../api";
import type { HelpCategory, HelpDocMeta } from "../../../api";
import { useTranslation } from "../../../i18n";
import HelpSearch from "./HelpSearch";

type Status = "loading" | "ready" | "error";

function docUrl(category: string, slug: string) {
  return `/help/${encodeURIComponent(category)}/${encodeURIComponent(slug)}`;
}

function byOrder(a: HelpDocMeta, b: HelpDocMeta) {
  return a.order - b.order || a.title.localeCompare(b.title);
}

function DocCard({ doc }: { doc: HelpDocMeta }) {
  return (
    <Link
      to={docUrl(doc.category, doc.slug)}
      data-testid={`help-doc-${doc.slug}`}
      className="group block h-full rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <PageCard className="mt-0 flex h-full flex-col gap-1.5 transition-colors duration-100 group-hover:border-border-strong group-hover:bg-surface-hover">
        <span className="flex items-start justify-between gap-3">
          <span className="text-sm font-medium text-foreground">{doc.title}</span>
          <ArrowRight
            className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-colors duration-100 group-hover:text-foreground"
            aria-hidden="true"
          />
        </span>
        {doc.description ? (
          <span className="text-xs leading-relaxed text-muted-foreground">{doc.description}</span>
        ) : null}
      </PageCard>
    </Link>
  );
}

export default function HelpHub() {
  const { t, lang } = useTranslation();

  const [docs, setDocs] = useState<HelpDocMeta[]>([]);
  const [categories, setCategories] = useState<HelpCategory[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [retryNonce, setRetryNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setStatus("loading");
    Promise.all([api.helpIndex(lang), api.helpCategories()])
      .then(([nextDocs, nextCategories]) => {
        if (cancelled) return;
        setDocs(nextDocs);
        setCategories(nextCategories);
        setStatus("ready");
      })
      .catch(() => {
        if (!cancelled) setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, [lang, retryNonce]);

  const groups = useMemo(() => {
    return categories
      .slice()
      .sort((a, b) => a.order - b.order)
      .map((category) => ({
        category,
        docs: docs.filter((doc) => doc.category === category.key).sort(byOrder),
      }))
      .filter((group) => group.docs.length > 0)
      .filter((group) => !(group.category.key === "start" && group.docs.length <= 3));
  }, [categories, docs]);

  const quickstart = useMemo(
    () => docs.filter((doc) => doc.category === "start").sort(byOrder).slice(0, 3),
    [docs],
  );

  const categoryLabel = (category: HelpCategory) =>
    category.labels[lang] || category.labels.en || category.key;

  return (
    <div className="h-full overflow-y-auto [scrollbar-gutter:stable]" data-testid="help-hub-scroll">
      <div className="flex min-h-full flex-col px-4 py-6 sm:px-8 sm:py-8">
        <PageShell
          title={t("help.hub_title", "Documentation")}
          description={t("help.hub_subtitle", "Browse guides, references, and answers for this workspace.")}
        >
          {status === "loading" ? (
            <PageCard className="mt-0 flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              {t("help.loading", "Loading documentation…")}
            </PageCard>
          ) : status === "error" ? (
            <PageCard className="mt-0 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-start gap-3">
                <AlertTriangle className="mt-0.5 size-5 shrink-0 text-warning-fg" aria-hidden="true" />
                <div>
                  <p className="text-sm font-medium text-foreground">
                    {t("help.hub_error_title", "Could not load the documentation")}
                  </p>
                  <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                    {t("help.hub_error_body", "Check your connection, then try again.")}
                  </p>
                </div>
              </div>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => setRetryNonce((value) => value + 1)}
              >
                {t("help.error_retry", "Retry")}
              </Button>
            </PageCard>
          ) : docs.length === 0 ? (
            <PageEmpty
              icon={<BookOpen />}
              title={t("help.hub_empty_title", "No documentation is installed")}
              detail={t("help.hub_empty_body", "This edition has no help docs yet.")}
            />
          ) : (
            <>
              <HelpSearch categories={categories} />

              {quickstart.length > 0 ? (
                <PageSection
                  title={t("help.hub_quickstart", "Quickstart")}
                  description={t("help.hub_quickstart_hint", "Start here if you are new.")}
                >
                  <div
                    className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3"
                    data-testid="help-quickstart"
                  >
                    {quickstart.map((doc) => (
                      <DocCard key={doc.slug} doc={doc} />
                    ))}
                  </div>
                </PageSection>
              ) : null}

              {groups.map((group) => (
                <PageSection
                  key={group.category.key}
                  id={`help-category-${group.category.key}`}
                  title={categoryLabel(group.category)}
                >
                  <div
                    className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3"
                    data-testid={`help-category-${group.category.key}`}
                  >
                    {group.docs.map((doc) => (
                      <DocCard key={doc.slug} doc={doc} />
                    ))}
                  </div>
                </PageSection>
              ))}
            </>
          )}
        </PageShell>
      </div>
    </div>
  );
}
