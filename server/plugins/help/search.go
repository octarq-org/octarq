package help

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/unicode/norm"
)

const (
	defaultHelpSearchLimit = 20
	maxHelpSearchLimit     = 50
	maxHelpSearchQuery     = 200
	helpSnippetRunes       = 160
	helpTitleWeight        = 5
	helpHeadingWeight      = 3
	helpBodyWeight         = 1
	helpExactTitleBoost    = 10
)

type searchCacheKey struct {
	orgID       uint
	lang        string
	fingerprint string
}

type helpSearchToken struct {
	norm  string
	start int
	end   int
}

type helpSearchSection struct {
	anchor        string
	anchorTitle   string
	plain         string
	tokens        []helpSearchToken
	counts        map[string]int
	headingCounts map[string]int
}

type helpSearchDoc struct {
	slug          string
	title         string
	category      string
	titleNorm     string
	titleCounts   map[string]int
	headingCounts map[string]int
	bodyCounts    map[string]int
	sections      []helpSearchSection
}

type helpSearchIndex struct {
	docs      []helpSearchDoc
	total     int
	dfTitle   map[string]int
	dfHeading map[string]int
	dfBody    map[string]int
}

type preparedHelpDoc struct {
	doc    plugin.HelpDoc
	source string
}

type HelpSearchSnippet struct {
	Text    string   `json:"text"`
	Offsets [][2]int `json:"offsets"`
}

type HelpSearchResult struct {
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Category    string            `json:"category"`
	Anchor      string            `json:"anchor"`
	AnchorTitle string            `json:"anchorTitle"`
	Snippet     HelpSearchSnippet `json:"snippet"`
	Score       float64           `json:"score"`
	URL         string            `json:"url"`
}

type HelpSearchResponse struct {
	Query   string             `json:"query"`
	Lang    string             `json:"lang"`
	Results []HelpSearchResult `json:"results"`
}

type HelpSearchInput struct {
	Ctx      huma.Context `hidden:"true"`
	Q        string       `query:"q"`
	Lang     string       `query:"lang"`
	Limit    int          `query:"limit"`
	Category string       `query:"category"`
}

func (i *HelpSearchInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type HelpSearchOutput struct {
	Body HelpSearchResponse
}

func (p *Plugin) searchDocs(ctx context.Context, input *HelpSearchInput) (*HelpSearchOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable. Try again.")
	}
	query := strings.TrimSpace(input.Q)
	if query == "" {
		return nil, huma.Error400BadRequest("Enter a search term.")
	}
	if len([]rune(query)) > maxHelpSearchQuery {
		return nil, huma.Error400BadRequest("Search term must be 200 characters or fewer.")
	}
	category := strings.TrimSpace(input.Category)
	if category != "" && !knownHelpCategory(category) {
		return nil, huma.Error400BadRequest("Choose a valid help category.")
	}

	r, _ := humago.Unwrap(input.Ctx)
	lang := input.Lang
	if lang == "" && r != nil {
		lang = r.Header.Get("Accept-Language")
	}
	lang = resolveHelpLang(lang)

	limit := input.Limit
	if limit < 0 {
		limit = 1
	}
	if limit == 0 {
		limit = defaultHelpSearchLimit
	}
	if limit > maxHelpSearchLimit {
		limit = maxHelpSearchLimit
	}

	orgID := p.requestOrgID(r)
	index := p.searchIndexFor(orgID, lang)
	return &HelpSearchOutput{
		Body: HelpSearchResponse{
			Query:   query,
			Lang:    lang,
			Results: index.search(query, category, limit),
		},
	}, nil
}

func knownHelpCategory(key string) bool {
	for _, category := range plugin.HelpCategories() {
		if category.Key == key {
			return true
		}
	}
	return false
}

func (p *Plugin) searchIndexFor(orgID uint, lang string) *helpSearchIndex {
	docs := p.getDocs(orgID, lang)
	prepared := prepareHelpDocs(docs, lang, p.placeholderValues(orgID))
	key := searchCacheKey{
		orgID:       orgID,
		lang:        lang,
		fingerprint: p.searchFingerprint(orgID, prepared),
	}
	if p != nil && p.searchIndexes != nil {
		if index, ok := p.searchIndexes.Get(key); ok {
			return index
		}
	}
	index := buildHelpSearchIndex(prepared)
	if p != nil {
		p.searchIndexes.Put(key, index)
	}
	return index
}

func prepareHelpDocs(docs []plugin.HelpDoc, lang string, values map[string]string) []preparedHelpDoc {
	prepared := make([]preparedHelpDoc, 0, len(docs))
	for _, doc := range docs {
		prepared = append(prepared, preparedHelpDoc{
			doc:    doc,
			source: substituteHelpPlaceholders(doc.Markdown, lang, values),
		})
	}
	return prepared
}

func (p *Plugin) searchFingerprint(orgID uint, prepared []preparedHelpDoc) string {
	h := sha256.New()
	if p != nil && p.pctx != nil && p.pctx.ActivePlugins != nil {
		names := make([]string, 0)
		for _, pl := range p.pctx.ActivePlugins() {
			active := false
			if p.pctx.PluginActive != nil {
				active = p.pctx.PluginActive(orgID, pl)
			}
			names = append(names, pl.Name()+":"+strconv.FormatBool(active))
		}
		sort.Strings(names)
		for _, name := range names {
			io.WriteString(h, name)
			h.Write([]byte{'\n'})
		}
	}
	h.Write([]byte{'\n'})
	for _, item := range prepared {
		io.WriteString(h, item.doc.Slug)
		h.Write([]byte{0})
		io.WriteString(h, item.doc.Title)
		h.Write([]byte{0})
		io.WriteString(h, item.doc.Category)
		h.Write([]byte{0})
		sum := sha256.Sum256([]byte(item.source))
		h.Write(sum[:])
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildHelpSearchIndex(prepared []preparedHelpDoc) *helpSearchIndex {
	index := &helpSearchIndex{
		docs:      make([]helpSearchDoc, 0, len(prepared)),
		total:     len(prepared),
		dfTitle:   make(map[string]int),
		dfHeading: make(map[string]int),
		dfBody:    make(map[string]int),
	}
	for _, item := range prepared {
		doc, source := parseMarkdown(item.source)
		sections, headings := extractHelpSections(doc, source, item.doc.Title)
		searchDoc := helpSearchDoc{
			slug:          item.doc.Slug,
			title:         item.doc.Title,
			category:      item.doc.Category,
			titleNorm:     normalizeHelpSearchText(item.doc.Title),
			titleCounts:   countHelpSearchTokens(tokenizeHelpSearchText(item.doc.Title)),
			headingCounts: make(map[string]int),
			bodyCounts:    make(map[string]int),
			sections:      sections,
		}
		for _, heading := range headings {
			mergeHelpSearchCounts(searchDoc.headingCounts, countHelpSearchTokens(tokenizeHelpSearchText(heading)))
		}
		for i := range searchDoc.sections {
			searchDoc.sections[i].tokens = tokenizeHelpSearchText(searchDoc.sections[i].plain)
			searchDoc.sections[i].counts = countHelpSearchTokens(searchDoc.sections[i].tokens)
			searchDoc.sections[i].headingCounts = countHelpSearchTokens(tokenizeHelpSearchText(searchDoc.sections[i].anchorTitle))
			mergeHelpSearchCounts(searchDoc.bodyCounts, searchDoc.sections[i].counts)
		}
		index.docs = append(index.docs, searchDoc)
	}
	for _, doc := range index.docs {
		for token := range doc.titleCounts {
			index.dfTitle[token]++
		}
		for token := range doc.headingCounts {
			index.dfHeading[token]++
		}
		for token := range doc.bodyCounts {
			index.dfBody[token]++
		}
	}
	return index
}

func extractHelpSections(doc ast.Node, source []byte, fallbackTitle string) ([]helpSearchSection, []string) {
	sections := make([]helpSearchSection, 0)
	headings := make([]string, 0)
	current := helpSearchSection{anchorTitle: fallbackTitle}
	flush := func() {
		if current.plain == "" && current.anchor == "" {
			return
		}
		if current.anchorTitle == "" {
			current.anchorTitle = fallbackTitle
		}
		sections = append(sections, current)
	}
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		if heading, ok := node.(*ast.Heading); ok {
			headingText := helpNodeText(heading, source)
			switch heading.Level {
			case 1:
				continue
			case 2, 3:
				flush()
				current = helpSearchSection{
					anchor:      helpHeadingAnchor(heading),
					anchorTitle: headingText,
				}
				headings = append(headings, headingText)
				continue
			}
		}
		blockText := helpNodeText(node, source)
		if blockText == "" {
			continue
		}
		if current.plain != "" {
			current.plain += " "
		}
		current.plain += blockText
	}
	flush()
	return sections, headings
}

func helpHeadingAnchor(heading *ast.Heading) string {
	value, ok := heading.AttributeString("id")
	if !ok {
		return ""
	}
	switch id := value.(type) {
	case []byte:
		return string(id)
	case string:
		return id
	default:
		return ""
	}
}

func helpNodeText(node ast.Node, source []byte) string {
	var b strings.Builder
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := n.(type) {
		case *ast.Text:
			b.Write(value.Value(source))
			if value.SoftLineBreak() || value.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(value.Value)
		case *ast.FencedCodeBlock:
			writeHelpCodeLines(&b, value.Lines(), source)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			writeHelpCodeLines(&b, value.Lines(), source)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

func writeHelpCodeLines(b *strings.Builder, lines *text.Segments, source []byte) {
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
		b.WriteByte(' ')
	}
}

func normalizeHelpSearchText(value string) string {
	return strings.ToLower(norm.NFKC.String(value))
}

func tokenizeHelpSearchText(value string) []helpSearchToken {
	runes := []rune(value)
	tokens := make([]helpSearchToken, 0, len(runes))
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case isHelpCJKRune(r):
			start := i
			for i < len(runes) && isHelpCJKRune(runes[i]) {
				i++
			}
			run := runes[start:i]
			for j := range run {
				tokens = append(tokens, helpSearchToken{
					norm:  normalizeHelpSearchText(string(run[j])),
					start: start + j,
					end:   start + j + 1,
				})
			}
			for j := 0; j+1 < len(run); j++ {
				tokens = append(tokens, helpSearchToken{
					norm:  normalizeHelpSearchText(string(run[j : j+2])),
					start: start + j,
					end:   start + j + 2,
				})
			}
		case isHelpWordRune(r):
			start := i
			for i < len(runes) && isHelpWordRune(runes[i]) {
				i++
			}
			token := normalizeHelpSearchText(string(runes[start:i]))
			if token != "" {
				tokens = append(tokens, helpSearchToken{norm: token, start: start, end: i})
			}
		default:
			i++
		}
	}
	return tokens
}

func isHelpCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}

func isHelpWordRune(r rune) bool {
	if isHelpCJKRune(r) {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mark, r)
}

func countHelpSearchTokens(tokens []helpSearchToken) map[string]int {
	counts := make(map[string]int)
	for _, token := range tokens {
		if token.norm == "" {
			continue
		}
		counts[token.norm]++
	}
	return counts
}

func mergeHelpSearchCounts(dst, src map[string]int) {
	for token, count := range src {
		dst[token] += count
	}
}

func queryHelpSearchTokens(query string) []string {
	seen := make(map[string]struct{})
	tokens := make([]string, 0)
	for _, token := range tokenizeHelpSearchText(query) {
		if token.norm == "" {
			continue
		}
		if _, ok := seen[token.norm]; ok {
			continue
		}
		seen[token.norm] = struct{}{}
		tokens = append(tokens, token.norm)
	}
	return tokens
}

func (index *helpSearchIndex) search(query, category string, limit int) []HelpSearchResult {
	results := make([]HelpSearchResult, 0)
	if index == nil {
		return results
	}
	tokens := queryHelpSearchTokens(query)
	queryNorm := normalizeHelpSearchText(strings.TrimSpace(query))
	for i := range index.docs {
		doc := &index.docs[i]
		if category != "" && doc.category != category {
			continue
		}
		score := 0.0
		for _, token := range tokens {
			score += helpFieldScore(doc.titleCounts[token], index.dfTitle[token], index.total, helpTitleWeight)
			score += helpFieldScore(doc.headingCounts[token], index.dfHeading[token], index.total, helpHeadingWeight)
			score += helpFieldScore(doc.bodyCounts[token], index.dfBody[token], index.total, helpBodyWeight)
		}
		if score <= 0 {
			continue
		}
		if queryNorm != "" && strings.Contains(doc.titleNorm, queryNorm) {
			score += helpExactTitleBoost
		}
		section := doc.bestSection(tokens)
		snippetText, offsets := buildHelpSnippet(section, tokens)
		if snippetText == "" {
			snippetText = doc.title
		}
		anchorTitle := section.anchorTitle
		if section.anchor == "" || anchorTitle == "" {
			anchorTitle = doc.title
		}
		results = append(results, HelpSearchResult{
			Slug:        doc.slug,
			Title:       doc.title,
			Category:    doc.category,
			Anchor:      section.anchor,
			AnchorTitle: anchorTitle,
			Snippet:     HelpSearchSnippet{Text: snippetText, Offsets: offsets},
			Score:       score,
			URL:         helpDocURL(doc.category, doc.slug, section.anchor),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Title != results[j].Title {
			return results[i].Title < results[j].Title
		}
		return results[i].Slug < results[j].Slug
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func helpFieldScore(tf, df, total int, weight float64) float64 {
	if tf <= 0 || df <= 0 || total <= 0 {
		return 0
	}
	return weight * float64(tf) * math.Log(1+float64(total)/float64(df))
}

func (doc *helpSearchDoc) bestSection(tokens []string) helpSearchSection {
	if len(doc.sections) == 0 {
		return helpSearchSection{anchorTitle: doc.title}
	}
	best := -1
	bestCount := 0
	for i := range doc.sections {
		count := 0
		for _, token := range tokens {
			count += doc.sections[i].counts[token]
			count += doc.sections[i].headingCounts[token]
		}
		if count > bestCount {
			bestCount = count
			best = i
		}
	}
	if best == -1 {
		return helpSearchSection{anchorTitle: doc.title, plain: doc.sections[0].plain}
	}
	return doc.sections[best]
}

func buildHelpSnippet(section helpSearchSection, tokens []string) (string, [][2]int) {
	runes := []rune(section.plain)
	if len(runes) == 0 {
		return section.anchorTitle, [][2]int{}
	}
	wanted := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		wanted[token] = struct{}{}
	}
	matches := make([]helpSearchToken, 0)
	for _, token := range section.tokens {
		if _, ok := wanted[token.norm]; ok {
			matches = append(matches, token)
		}
	}
	start, end := helpSnippetWindow(len(runes), matches)
	return string(runes[start:end]), helpSnippetOffsets(matches, start, end)
}

func helpSnippetWindow(length int, matches []helpSearchToken) (int, int) {
	if length <= helpSnippetRunes {
		return 0, length
	}
	if len(matches) == 0 {
		return 0, helpSnippetRunes
	}
	bestStart := matches[0].start
	bestCount := -1
	for i := range matches {
		windowEnd := matches[i].start + helpSnippetRunes
		count := 0
		for j := i; j < len(matches) && matches[j].start < windowEnd; j++ {
			count++
		}
		if count > bestCount {
			bestCount = count
			bestStart = matches[i].start
		}
	}
	start := bestStart - 20
	if start < 0 {
		start = 0
	}
	end := start + helpSnippetRunes
	if end > length {
		end = length
	}
	return start, end
}

func helpSnippetOffsets(matches []helpSearchToken, start, end int) [][2]int {
	ranges := make([][2]int, 0, len(matches))
	for _, match := range matches {
		if match.start < start || match.end > end {
			continue
		}
		ranges = append(ranges, [2]int{match.start - start, match.end - start})
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i][0] != ranges[j][0] {
			return ranges[i][0] < ranges[j][0]
		}
		return ranges[i][1] < ranges[j][1]
	})
	merged := make([][2]int, 0, len(ranges))
	for _, current := range ranges {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}
		last := &merged[len(merged)-1]
		if current[0] <= last[1] {
			if current[1] > last[1] {
				last[1] = current[1]
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func helpDocURL(category, slug, anchor string) string {
	target := "/help/" + url.PathEscape(category) + "/" + url.PathEscape(slug)
	if anchor != "" {
		target += "#" + url.PathEscape(anchor)
	}
	return target
}
