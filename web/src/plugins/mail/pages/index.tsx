import { useEffect, useState, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Email, Mailbox } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Button } from "../../../ui";
import { Inbox, Send, Plus } from "lucide-react";
import { MailSettings } from "./MailSettings";
import { SMTPSenders } from "./SMTPSenders";
import { SuppressionList } from "./SuppressionList";
import { useTranslation } from "../../../i18n";
import { ReplyDraft, MailFolder } from "./types";
import { EmailViewForm } from "./EmailView";
import { MailboxEditor } from "./MailboxEditor";
import { Compose } from "./Compose";
import { FolderNav } from "./FolderNav";
import { MailboxBar } from "./MailboxBar";
import { MailGuide } from "./MailGuide";
import { EmailListPane } from "./EmailListPane";
import { MailTabs, MailTab } from "./MailTabs";

export default function MailPage() {
  const [boxes, setBoxes] = useState<Mailbox[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [active, setActive] = useState<number | undefined>(undefined);
  const [folder, setFolder] = useState<MailFolder>("inbox");
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
      if (tTab === "mail") prev.delete("tab");
      else prev.set("tab", tTab);
      return prev;
    }, { replace: true });
  };

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
      const res = await mailApi.emails(active, { q, folder, limit, offset });
      setHasMore(res.length >= limit);
      setEmails(prev => (reset ? res : [...prev, ...res]));
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
    }, 150);
    return () => clearTimeout(timer);
  }, [active, folder, q]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadEmails();
  };

  async function openEmail(e: Email) {
    if (folder === "drafts") {
      setCompose({ id: e.id, to: e.to, subject: e.subject, text: e.text });
      return;
    }
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

      <MailTabs tab={tab} onTabChange={setTab} />

      {tab === "mail" && (
        <div className="space-y-4">
          {boxes.length === 0 && <MailGuide />}

          <MailboxBar
            boxes={boxes}
            active={active}
            totalUnread={totalUnread}
            activeBox={activeBox}
            onSelect={setActive}
            onEdit={setEditBox}
          />

          <FolderNav currentFolder={folder} onSelectFolder={(f) => { setFolder(f); setOpen(null); }} />

          <div className="grid grid-cols-1 lg:grid-cols-[360px_1fr] gap-6 min-h-0 items-start">
            <EmailListPane
              emails={emails}
              loading={loading}
              folder={folder}
              q={q}
              onSearchChange={setQ}
              openEmailId={open?.id}
              onOpenEmail={openEmail}
              onScroll={handleScroll}
              boxes={boxes}
              activeMailboxId={active}
              onNewBox={() => setNewBox(true)}
            />

            <div className="w-full">
              {open ? (
                <EmailViewForm
                  email={open}
                  onClose={() => setOpen(null)}
                  onReply={(d) => { setOpen(null); setCompose(d); }}
                  onEditDraft={(d) => { setOpen(null); setCompose(d); }}
                  onChanged={() => { loadEmails(true); loadBoxes(); }}
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

      {tab === "inbound" && <GlassCard className="p-6"><MailSettings /></GlassCard>}
      {tab === "smtp" && <GlassCard className="p-6"><SMTPSenders /></GlassCard>}
      {tab === "suppressions" && <GlassCard className="p-6"><SuppressionList /></GlassCard>}

      {newBox && <MailboxEditor box={null} hosts={mailHostOptions} onClose={() => setNewBox(false)} onSaved={() => { setNewBox(false); loadBoxes(); }} />}
      {editBox && <MailboxEditor box={editBox} hosts={mailHostOptions} onClose={() => setEditBox(null)} onSaved={() => { setEditBox(null); loadBoxes(); loadEmails(true); }} />}
      {compose && (
        <Compose
          draft={compose === true ? undefined : compose}
          onClose={() => setCompose(null)}
          onDraftSaved={() => { loadEmails(true); }}
        />
      )}
    </ScreenWrap>
  );
}
