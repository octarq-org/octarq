import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../../../api";
import { PageHeader } from "../../../ui";
import { useTranslation } from "../../../i18n";

interface DocMeta {
  slug: string;
  title: string;
  group: string;
  order: number;
}

interface DocContent {
  title: string;
  html: string;
}

export default function HelpViewer() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentSlug = searchParams.get("doc");

  const [docs, setDocs] = useState<DocMeta[]>([]);
  const [loadingDocs, setLoadingDocs] = useState(true);

  const [content, setContent] = useState<DocContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.helpIndex()
      .then((res) => {
        setDocs(res.body || []);
        if (!currentSlug && res.body?.length > 0) {
          setSearchParams({ doc: res.body[0].slug }, { replace: true });
        }
      })
      .catch((err) => console.error("failed to load docs", err))
      .finally(() => setLoadingDocs(false));
  }, []);

  useEffect(() => {
    if (!currentSlug) return;
    setLoadingContent(true);
    setError("");
    api.helpPage(currentSlug)
      .then((res) => setContent(res.body))
      .catch((err: any) => setError(err.message || "Failed to load documentation"))
      .finally(() => setLoadingContent(false));
  }, [currentSlug]);

  const groups = Array.from(new Set(docs.map((d) => d.group)));

  return (
    <div className="space-y-6 h-full flex flex-col">
      <PageHeader title={t("help.title")} />
      <div className="flex flex-1 overflow-hidden min-h-[500px]">
        {/* Left rail */}
        <div className="w-64 border-r border-border overflow-y-auto p-4 pr-6">
          {loadingDocs ? (
            <div className="p-8 text-sm text-center text-foreground/50">{t("help.loading")}</div>
          ) : docs.length === 0 ? (
            <div className="text-muted-foreground text-sm">{t("help.empty")}</div>
          ) : (
            <div className="space-y-6">
              {groups.map((group) => (
                <div key={group}>
                  <h3 className="font-medium text-sm text-muted-foreground mb-2 px-2 uppercase tracking-wider">{t(`help.group.${group}`, group)}</h3>
                  <div className="space-y-1">
                    {docs
                      .filter((d) => d.group === group)
                      .map((d) => (
                        <button
                          key={d.slug}
                          onClick={() => setSearchParams({ doc: d.slug })}
                          className={`w-full text-left px-3 py-1.5 rounded-md text-sm transition-colors ${
                            currentSlug === d.slug
                              ? "bg-primary/10 text-primary font-medium"
                              : "text-foreground hover:bg-surface-hover"
                          }`}
                        >
                          {d.title}
                        </button>
                      ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Main pane */}
        <div className="flex-1 overflow-y-auto p-8">
          {loadingContent ? (
            <div className="flex items-center justify-center h-full">
              <div className="p-8 text-sm text-center text-foreground/50">{t("help.loading")}</div>
            </div>
          ) : error ? (
            <div className="text-destructive">{error}</div>
          ) : content ? (
            <div className="max-w-3xl">
              <h1 className="text-3xl font-semibold tracking-tight mb-8">{content.title}</h1>
              <div 
                className="prose prose-sm md:prose-base prose-slate dark:prose-invert max-w-none"
                dangerouslySetInnerHTML={{ __html: content.html }}
              />
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
