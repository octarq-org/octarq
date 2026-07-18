import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Attachment, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle } from "lucide-react";
import { MailSettings } from "./MailSettings";
import { SMTPSenders } from "./SMTPSenders";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";
import { EmailViewForm, AuthBadges } from "./EmailView";
import { MailboxEditor } from "./MailboxEditor";
import { Compose } from "./Compose";

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
  const tabParam = searchParams.get("tab");
  const tab = (tabParam === "settings") ? "settings" : "mail";
  const setTab = (t: "mail" | "settings") => {
    setSearchParams(prev => {
      if (t === "mail") {
        prev.delete("tab");
      } else {
        prev.set("tab", t);
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
    const t = setTimeout(() => {
      loadEmails(true);
    }, 200);
    return () => clearTimeout(t);
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

      <div className="flex gap-0 border-b border-white/[0.06] mb-6">
        <button
          onClick={() => setTab('mail')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
            tab === 'mail'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("mail.tabMail")}
        </button>
        <button
          onClick={() => setTab('settings')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 ${
            tab === 'settings'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("mail.tabSettings")}
        </button>
      </div>

      {tab === 'mail' && (<>

      {boxes.length === 0 && (
        <Guide title={t("mail.guideTitle")} open>
          <ol className="ml-4 list-decimal space-y-1.5 text-sm leading-relaxed text-white/70">
            <li>{t("mail.guideStep1Pre")}<b>{t("mail.guideStep1Domains")}</b>{t("mail.guideStep1Mid")}<b>{t("mail.guideStep1AcceptEmail")}</b>{t("mail.guideStep1Post")}</li>
            <li>{t("mail.guideStep2Pre")}<b>{t("mail.guideStep2Routing")}</b>{t("mail.guideStep2Post")}</li>
            <li>{t("mail.guideStep3Pre")}<Code>deploy/cloudflare-email-worker.js</Code>{t("mail.guideStep3Mid1")}<Code>OCTARQ_ENDPOINT</Code>{t("mail.guideStep3Mid2")}<b>{t("mail.guideStep3WebhookUrl")}</b>{t("mail.guideStep3Mid3")}<b>{t("mail.guideStep3SettingsMailboxes")}</b>{t("mail.guideStep3Post")}</li>
            <li>{t("mail.guideStep4")}</li>
          </ol>
        </Guide>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left column - list */}
        <div className="flex flex-col min-h-0 w-full">
          <div className="mb-3 flex items-center gap-2">
            <div className="min-w-0 flex-1">
              <Select
                value={active === undefined ? "" : String(active)}
                onValueChange={(v) => setActive(v ? Number(v) : undefined)}
                options={[
                  { value: "", label: t("mail.allMailboxes") },
                  ...boxes.map((b) => ({
                    value: String(b.id),
                    label: `${b.address}${b.unread > 0 ? ` (${b.unread})` : ""}`,
                  })),
                ]}
              />
            </div>
            {active !== undefined && (
              <Button
                variant="ghost"
                title={t("mail.editMailbox")}
                onClick={() => setEditBox(boxes.find(b => b.id === active) || null)}
                className="shrink-0 p-2"
              >
                <Settings className="h-4 w-4" />
              </Button>
            )}
          </div>
          
          <div className="mb-3">
            <input
              className="input w-full"
              placeholder={t("mail.searchPlaceholder")}
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          
          <GlassCard className="overflow-hidden">
            <div className="overflow-y-auto max-h-[600px] divide-y divide-white/[0.04]" onScroll={handleScroll}>
              {emails.length === 0 && !loading ? (
                <div className="p-8 text-center text-white/40 text-sm">{t("mail.noMessages")}</div>
              ) : (
                <>
                  {emails.map((e) => (
                    <button
                      key={e.id}
                      className={`flex w-full flex-col p-4 text-left hover:bg-white/[0.03] transition-colors ${open?.id === e.id ? "bg-white/[0.05]" : ""}`}
                      onClick={() => openEmail(e)}
                    >
                      <div className="flex items-center justify-between w-full mb-1 gap-2">
                        <div className="flex items-center gap-2 overflow-hidden">
                          {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                          <span className={`truncate text-sm ${e.read ? "text-white/55" : "font-semibold text-white"}`}>{e.from || t("mail.unknownSender")}</span>
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0 ml-2">
                          <AuthBadges spf={e.authSpf} dkim={e.authDkim} dmarc={e.authDmarc} compact />
                          <span className="text-[11px] text-white/40">{timeAgo(e.receivedAt)}</span>
                        </div>
                      </div>
                      <div className={`truncate text-xs ${e.read ? "text-white/40" : "text-white/70"}`}>
                        {e.subject || t("mail.noSubject")}
                      </div>
                    </button>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-white/40">{t("mail.loading")}</div>}
                </>
              )}
            </div>
          </GlassCard>
        </div>

        {/* Right column - email view pane */}
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
            <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center text-white/40 border border-white/[0.04]/40">
              <Inbox className="h-10 w-10 mb-2 opacity-50 text-indigo-400" />
              <p className="text-sm">{t("mail.selectEmail")}</p>
            </GlassCard>
          )}
        </div>
      </div>

      </>
      )}
      {tab === 'settings' && (
        <div className="space-y-8">
          <div>
            <h3 className="text-sm font-semibold text-white/70 mb-3">{t("mail.inboundConfig")}</h3>
            <GlassCard className="p-6"><MailSettings /></GlassCard>
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/70 mb-3">{t("mail.smtpGateways")}</h3>
            <GlassCard className="p-6"><SMTPSenders /></GlassCard>
          </div>
        </div>
      )}

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

