with open("internal/server/server.go", "r") as f:
    content = f.read()

content = content.replace('"github.com/octarq-org/octarq/internal/shortlink"\n', "")
content = content.replace("short *shortlink.Service, ", "")
content = content.replace("short:  short,\n", "")
content = content.replace("	short        *shortlink.Service\n", "")

content = content.replace("""	// 4. Everything else in the root namespace is a short link.
	if r.Method == http.MethodGet {
		slug := strings.TrimPrefix(path, "/")
		if slug != "" && !strings.Contains(slug, "/") {
			if link, ok := s.short.Lookup(r.Host, slug); ok {
				s.short.Handle(w, r, link)
				return
			}
		}
	}
	http.NotFound(w, r)""", """	// 4. Everything else falls back to the core API mux (which handles /api/ and root shortlinks).
	s.api.ServeHTTP(w, r)""")

with open("internal/server/server.go", "w") as f:
    f.write(content)
