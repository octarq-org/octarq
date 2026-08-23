import { useEffect, useState, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Attachment, Email, Mailbox } from "../api";
import { Code, Empty, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle, Search, ShieldCheck, ArrowRight, Edit3, Filter } from "lucide-react";
import { MailSettings } from "./MailSettings";
import { SMTPSenders } from "./SMTPSenders";
import { SuppressionList } from "./SuppressionList";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";
import { EmailViewForm, AuthBadges } from "./EmailView";
import { MailboxEditor } from "./MailboxEditor";
import { Compose } from "./Compose";
import { ListSkeleton } from "../../../components/ListSkeleton";

type MailTab = "mail" | "inbound" | "smtp" | "suppressions";

export default function MailPage() {
  const [boxes, setBoxes] = useState<Mailbox[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [active, setActive] = useState<number | undefined>(undefined);
  const [emails, setEmails] = useState<Email[]>([]);
  const [open, setOpen] = useState<Email | null>(null);
  const [newBox, setNewBox] = useState(false);
  const [editBox, setEditBox] = useState<Mailbox | null>(null);
  const [compose, setCompose] = useState<ReplyDraft | true | null>(null);

  const [q, setQ] = useState("");
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

  const [searchParams, setSearchParams] = useSearchParams();
  const { t } = useTranslation();

  useEffect(() => {
    if (searchParams.get("create") === "1") {
      setNewBox(true);
      setSearchParams(prev => {
        const next = new URLSearchParams(prev);
        next.delete("create");
        return next;
      }, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const tabParam = searchParams.get("tab");
  const tab: MailTab = useMemo(() => {
    if (tabParam === "settings" || tabParam === "inbound") return "inbound";
    if (tabParam === "smtp") return "smtp";
    if (tabParam === "suppressions") return "suppressions";
    return "mail";
  }, [tabParam]);

  const setTab = (tTab: MailTab) => {
    setSearchParams(prev => {
      if (tTab === "mail") {
        prev.delete("tab");
      } else {
        prev.set("tab", tTab);
      }
      return prev;
    }, { replace: true });
  };

  // Every mail host across all mail-enabled domains (incl. subdomains).
  const mailHostOptions = Array.from(new Set(domains.flatMap(effectiveMailHosts)));

  async function loadBoxes() {
    setBoxes(await mailApi.mailboxes());
  }

  async function loadEmails(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await mailApi.emails(active, { q, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setEmails(prev => reset ? res : [...prev, ...res]);
      setPage(reset ? 1 : page + 1);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadBoxes();
    api.domains().then(setDomains).catch(() => {});
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => {
      loadEmails(true);
    }, 200);
    return () => clearTimeout(timer);
  }, [active, q]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadEmails();
  };

  async function openEmail(e: Email) {
    const full = await mailApi.email(e.id);
    setOpen(full);
    loadEmails(true);
    loadBoxes();
  }

  async function markAllRead() {
    await mailApi.readAllEmails(active);
    loadEmails(true);
    loadBoxes();
  }

  const activeBox = useMemo(() => boxes.find(b => b.id === active), [boxes, active]);
  const totalUnread = useMemo(() => boxes.reduce((acc, b) => acc + (b.unread || 0), 0), [boxes]);

  return (
    <ScreenWrap>
      <PageHeader
        title={t("mail.pageTitle")}
        description={t("mail.pageDesc")}
        action={
          <div className="flex gap-2">
            <Button variant="ghost" onClick={markAllRead} className="py-1.5 text-xs">
              {t("mail.markRead")}
            </Button>
            <Button variant="outline" onClick={() => setCompose(true)} className="gap-1.5 py-1.5 text-xs">
              <Send className="h-3.5 w-3.5" />
              {t("mail.compose")}
            </Button>
            <Button variant="primary" onClick={() => setNewBox(true)} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              {t("mail.newMailbox")}
            </Button>
          </div>
        }
      />

      {/* Top Level Navigation Tabs */}
      <div className="flex gap-0 border-b border-foreground/[0.06] mb-6 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
        <button
          onClick={() => setTab("mail")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "mail"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("mail.tabMail")}
        </button>
        <button
          onClick={() => setTab("inbound")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "inbound"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("mail.tabInbound")}
        </button>
        <button
          onClick={() => setTab("smtp")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "smtp"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("mail.tabSmtp")}
        </button>
        <button
          onClick={() => setTab("suppressions")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "suppressions"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("mail.tabSuppressions")}
        </button>
      </div>

      {tab === "mail" && (
        <div className="space-y-4">
          {boxes.length === 0 && (
            <Guide title={t("mail.guideTitle")} open>
              <ol className="ml-4 list-decimal space-y-1.5 text-sm leading-relaxed text-foreground/70">
                <li>{t("mail.guideStep1Pre")}<b>{t("mail.guideStep1Domains")}</b>{t("mail.guideStep1Mid")}<b>{t("mail.guideStep1AcceptEmail")}</b>{t("mail.guideStep1Post")}</li>
                <li>{t("mail.guideStep2Pre")}<b>{t("mail.guideStep2Routing")}</b>{t("mail.guideStep2Post")}</li>
                <li>{t("mail.guideStep3Pre")}<Code>deploy/cloudflare-email-worker.js</Code>{t("mail.guideStep3Mid1")}<Code>OCTARQ_ENDPOINT</Code>{t("mail.guideStep3Mid2")}<b>{t("mail.guideStep3WebhookUrl")}</b>{t("mail.guideStep3Mid3")}<b>{t("mail.guideStep3SettingsMailboxes")}</b>{t("mail.guideStep3Post")}</li>
                <li>{t("mail.guideStep4")}</li>
              </ol>
            </Guide>
          )}

          {/* Mailbox Selector Strip */}
          {boxes.length > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-3 bg-well p-2 rounded-2xl border border-foreground/[0.05]">
              <div className="flex items-center gap-1.5 flex-wrap flex-1">
                <button
                  type="button"
                  onClick={() => setActive(undefined)}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium transition-colors cursor-pointer ${
                    active === undefined
                      ? "bg-accent text-accent-fg shadow-sm font-semibold"
                      : "text-foreground/60 hover:text-foreground hover:bg-foreground/[0.03]"
                  }`}
                >
                  <Inbox className="h-3.5 w-3.5" />
                  <span>{t("mail.allMailboxes")}</span>
                  {totalUnread > 0 && (
                    <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${active === undefined ? "bg-accent-fg/20 text-accent-fg" : "bg-primary text-primary-fg"}`}>
                      {totalUnread}
                    </span>
                  )}
                </button>

                {boxes.map((b) => (
                  <button
                    key={b.id}
                    type="button"
                    onClick={() => setActive(b.id)}
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-mono transition-colors cursor-pointer ${
                      active === b.id
                        ? "bg-accent text-accent-fg shadow-sm font-semibold"
                        : "text-foreground/70 hover:text-foreground hover:bg-foreground/[0.03]"
                    }`}
                  >
                    <span>{b.address}</span>
                    {b.unread > 0 && (
                      <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${active === b.id ? "bg-accent-fg/20 text-accent-fg" : "bg-primary text-primary-fg font-sans font-bold"}`}>
                        {b.unread}
                      </span>
                    )}
                  </button>
                ))}
              </div>

              {activeBox && (
                <Button
                  variant="subtle"
                  className="text-xs py-1 px-2.5 gap-1 shrink-0"
                  onClick={() => setEditBox(activeBox)}
                  title={t("mail.editMailbox")}
                >
                  <Settings className="h-3.5 w-3.5" />
                  <span>{t("mail.editMailbox")}</span>
                </Button>
              )}
            </div>
          )}

          {/* 2-Pane Split Mail Workspace */}
          <div className="grid grid-cols-1 lg:grid-cols-[360px_1fr] gap-6 min-h-0 items-start">
            {/* Left column: Search and email list */}
            <div className="flex flex-col min-h-0 w-full">
              <div className="mb-3 relative">
                <input
                  className="input w-full !pl-8 text-sm"
                  placeholder={t("mail.searchPlaceholder")}
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                />
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-foreground/45 pointer-events-none" />
                {q && (
                  <button
                    type="button"
                    onClick={() => setQ("")}
                    className="absolute right-2.5 top-2.5 text-foreground/40 hover:text-foreground p-0.5 cursor-pointer"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>

              <GlassCard className="overflow-hidden">
                {emails.length === 0 && loading ? (
                  <ListSkeleton rows={7} ariaLabel={t("mail.loading")} />
                ) : (
                  <div className="overflow-y-auto max-h-[650px] divide-y divide-foreground/[0.04]" onScroll={handleScroll}>
                    {emails.length === 0 ? (
                      q ? (
                        <div className="flex flex-col items-center gap-3 px-4 py-8 text-center">
                          <p className="text-sm text-foreground/60">
                            {t("mail.emptyFilteredReason")} <span className="font-mono text-foreground/80">{`“${q}”`}</span>
                          </p>
                          <Button variant="ghost" className="text-xs py-1.5" onClick={() => setQ("")}>
                            {t("mail.clearSearch")}
                          </Button>
                        </div>
                      ) : boxes.length === 0 ? (
                        <Empty
                          reason={t("mail.emptyNoBoxesReason")}
                          detail={t("mail.emptyNoBoxesDetail")}
                          action={<Button variant="primary" className="mt-1 text-xs py-1.5" onClick={() => setNewBox(true)}>{t("mail.newMailbox")}</Button>}
                        >
                          <MailIcon className="h-8 w-8 text-foreground/50 mb-1" />
                        </Empty>
                      ) : (
                        <Empty
                          reason={<>{t("mail.emptyInboxReasonPre")} <span className="font-mono">{`“${boxes.find(b => b.id === active)?.address ?? boxes[0]?.address}”`}</span></>}
                          detail={t("mail.emptyInboxDetail")}
                        >
                          <Inbox className="h-8 w-8 text-foreground/50 mb-1" />
                        </Empty>
                      )
                    ) : (
                      <>
                        {emails.map((e) => (
                          <button
                            key={e.id}
                            className={`flex w-full flex-col p-4 text-left hover:bg-foreground/[0.03] transition-colors cursor-pointer ${
                              open?.id === e.id ? "bg-foreground/[0.06] border-l-2 border-primary" : ""
                            }`}
                            onClick={() => openEmail(e)}
                          >
                            <div className="flex items-center justify-between w-full mb-1 gap-2">
                              <div className="flex items-center gap-2 min-w-0">
                                {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-primary" />}
                                <span className={`truncate text-sm ${e.read ? "text-foreground/60" : "font-bold text-foreground"}`}>
                                  {e.from || t("mail.unknownSender")}
                                </span>
                              </div>
                              <div className="flex items-center gap-1.5 shrink-0 ml-2">
                                <AuthBadges spf={e.authSpf} dkim={e.authDkim} dmarc={e.authDmarc} compact />
                                <span className="text-[11px] text-foreground/40">{timeAgo(e.receivedAt)}</span>
                              </div>
                            </div>
                            <div className={`truncate text-xs ${e.read ? "text-foreground/45" : "text-foreground/80 font-medium"}`}>
                              {e.subject || t("mail.noSubject")}
                            </div>
                          </button>
                        ))}
                        {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("mail.loading")}</div>}
                      </>
                    )}
                  </div>
                )}
              </GlassCard>
            </div>

            {/* Right column: Email View reading pane */}
            <div className="w-full">
              {open ? (
                <EmailViewForm
                  email={open}
                  onClose={() => setOpen(null)}
                  onReply={(d) => {
                    setOpen(null);
                    setCompose(d);
                  }}
                  onChanged={() => {
                    loadEmails(true);
                    loadBoxes();
                  }}
                />
              ) : (
                <GlassCard className="flex flex-col items-center justify-center py-16 px-6 text-center text-foreground/40 border border-foreground/[0.04]/40">
                  <Inbox className="h-12 w-12 mb-3 opacity-40 text-accent-fg" />
                  <p className="text-sm font-medium">{t("mail.selectEmail")}</p>
                </GlassCard>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Inbound Webhook Tab */}
      {tab === "inbound" && (
        <GlassCard className="p-6">
          <MailSettings />
        </GlassCard>
      )}

      {/* SMTP Senders Tab */}
      {tab === "smtp" && (
        <GlassCard className="p-6">
          <SMTPSenders />
        </GlassCard>
      )}

      {/* Suppression List Tab */}
      {tab === "suppressions" && (
        <GlassCard className="p-6">
          <SuppressionList />
        </GlassCard>
      )}

      {/* Modals */}
      {newBox && (
        <MailboxEditor
          box={null}
          hosts={mailHostOptions}
          onClose={() => setNewBox(false)}
          onSaved={() => {
            setNewBox(false);
            loadBoxes();
          }}
        />
      )}
      {editBox && (
        <MailboxEditor
          box={editBox}
          hosts={mailHostOptions}
          onClose={() => setEditBox(null)}
          onSaved={() => {
            setEditBox(null);
            loadBoxes();
            loadEmails(true);
          }}
        />
      )}
      {compose && (
        <Compose draft={compose === true ? undefined : compose} onClose={() => setCompose(null)} />
      )}
    </ScreenWrap>
  );
}
