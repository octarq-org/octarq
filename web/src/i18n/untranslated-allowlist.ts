/**
 * Allowlist for keys in non-English dictionaries whose values are intentionally identical to English
 * (e.g. brand names, technical protocols, acronyms, universal placeholders, or words with identical target-language spelling).
 * Each entry must include a one-line comment explaining the reason for exemption.
 */
export const UNTRANSLATED_ALLOWLIST = new Set<string>([
  // Product & brand names
  "footer.github", // Brand name (GitHub)
  "settings.googleClientId", // Brand name & technical credential identifier
  "settings.googleClientSecret", // Brand name & technical credential identifier
  "settings.githubClientId", // Brand name & technical credential identifier
  "settings.githubClientSecret", // Brand name & technical credential identifier
  "settings.telegramChatId", // Brand name & technical identifier

  // Technical protocols, acronyms & standards
  "nav.domains", // Technical protocol acronym (DNS)
  "nav.webhooks", // Standard technical term (Webhooks)
  "settings.webhooksTitle", // Standard technical term (Webhooks)
  "settings.customHttpTargetUrl", // Standard technical term (Webhook URL)
  "settings.pluginCategory.ai", // Technical acronym (AI)
  "settings.instanceRlAuth", // Technical acronyms (Auth RPM / IP)
  "settings.instanceRlApi", // Technical acronyms (API RPM / IP)
  "abuse.ip", // Technical acronym (IP)
  "abuse.reasonSpam", // Standard technical term (Spam)
  "abuse.reasonPhishing", // Standard cybersecurity term (Phishing)
  "abuse.reasonMalware", // Standard cybersecurity term (Malware)
  "personal.tokenScopeAdmin", // Scope identifier token (admin)
  "status.statusNa", // Standard universal abbreviation (N/A)
  "command.chat.tokens", // Standard technical unit (tokens)

  // Example inputs & placeholders
  "app.emailPlaceholder", // Example email placeholder (you@domain.com)
  "personal.newEmailPlaceholder", // Example email placeholder (you@domain.com)
  "settings.apiKeysPlaceholderNew", // Technical input placeholder (API token...)
  "settings.botAuthToken", // Technical credential placeholder

  // Industry terms & loanwords standard in ES/PT tech communities
  "groups.Marketing", // Industry standard term in ES/PT (Marketing)
  "settings.pluginCategory.marketing", // Industry standard term in ES/PT (Marketing)
  "nav.links", // Standard term in PT tech UI (Links)
  "settings.instancePluginName", // Loanword standard in ES/PT (Plugin)
  "settings.buildTitle", // "Build" is the standard loanword in PT tech UI
  "settings.buildCommit", // Git term used verbatim in ES/PT (Commit)
  "uiCommon.pluginId", // Loanword standard in PT (Plugin)

  // Words with identical spelling in the target language
  "groups.Personal", // Identical spelling in Spanish (Personal)
  "nav.general", // Identical spelling in Spanish (General)
  "settings.generalTitle", // Identical spelling in Spanish (General)
  "audit.colActor", // Identical spelling in Spanish (Actor)

  // Protocol acronyms that are universal
  "uiCommon.formErrorStatus", // Technical acronym (HTTP)
]);
