with open("app/app.go", "r") as f:
    content = f.read()

content = content.replace('"github.com/octarq-org/octarq/internal/shortlink"\n', "")
content = content.replace("shortlink.SetTrustProxy(a.cfg.TrustProxy)\n\tshort := shortlink.New(a.gdb, a.geo).WithCache(a.auth.Cache())\n", "")
content = content.replace("server.New(a.cfg, api.CSRFGuard(mux), short, webFS", "server.New(a.cfg, api.CSRFGuard(mux), webFS")

with open("app/app.go", "w") as f:
    f.write(content)
