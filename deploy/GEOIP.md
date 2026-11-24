# GeoIP (GeoLite2) setup

octarq resolves a click's IP into country / region / city for the analytics
breakdowns. This is **optional** — without a database, geo columns are simply
empty and everything else works. The lookup uses MaxMind's free **GeoLite2-City**
database (an `.mmdb` file).

octarq does not ship or download the database. Point **`OCTARQ_GEOIP_DB`** at an
`.mmdb` you obtained yourself. With it unset, geo lookups are disabled.

Behind a trusted proxy, octarq can also take the client country from the
`CF-IPCountry` header instead of the database. See
[Country from a trusted proxy](#country-from-a-trusted-proxy).

## 1. Get GeoLite2-City.mmdb

The community mirrors need **no account and no license key** and are the easiest
way to obtain the file.

Several projects republish the free GeoLite2 databases, auto-updated by GitHub
Actions on MaxMind's own Tuesday/Friday schedule:

```bash
# jsDelivr CDN (gzipped — gunzip after download):
curl -L https://cdn.jsdelivr.net/npm/geolite2-city/GeoLite2-City.mmdb.gz \
  | gunzip > GeoLite2-City.mmdb

# or the P3TERX release mirror (already an .mmdb, no gunzip):
curl -Lo GeoLite2-City.mmdb \
  https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb
```

These mirrors redistribute MaxMind's data under its license (GeoLite2 is offered
under CC BY-SA 4.0); that restriction is why **octarq** itself does not bundle the
file.

## 2. Wire it into your deployment

### Bare binary / local

```bash
OCTARQ_GEOIP_DB=/path/to/GeoLite2-City.mmdb ./octarq
```

### Docker Compose — bind mount

Keep the file on the host and mount it read-only (see the commented block in
[`docker-compose.yml`](../docker-compose.yml)):

```yaml
environment:
  OCTARQ_GEOIP_DB: /geoip/GeoLite2-City.mmdb
volumes:
  - /abs/host/path/GeoLite2-City.mmdb:/geoip/GeoLite2-City.mmdb:ro
```

### Kubernetes — bake it into a private image (recommended)

A host bind mount doesn't translate to k8s, and the database is far too big for a
ConfigMap (1 MB limit). The clean answer is to **bake it into your own private
image** with [`deploy/Dockerfile.geoip`](Dockerfile.geoip):

```bash
docker build -t octarq:base .                              # base image, no DB (may be public)
docker build -f deploy/Dockerfile.geoip \
  --build-arg BASE=octarq:base \
  -t registry.example.com/octarq:geoip .                   # private image WITH the DB
```

That image sets `OCTARQ_GEOIP_DB` itself, so the Deployment needs no volume and no
extra env. GeoLite2 is CC BY-SA 4.0, so redistributing the baked image is allowed
**with attribution**; the simplest path is to **keep it in your private registry**
and skip that burden.

## 3. Country from a trusted proxy

When octarq runs behind Cloudflare or another proxy that sets `CF-IPCountry`, you
can use that header instead of the mmdb for session locations:

```bash
OCTARQ_GEOIP_DB=            # optional; leave empty to disable the mmdb
OCTARQ_TRUST_PROXY=true
```

The header is spoofable. octarq reads it **only** when `OCTARQ_TRUST_PROXY` is
true, the value is a two-letter ISO code, and the value is not `XX`. Values that
fail these checks are ignored and the mmdb resolver is used. Never enable
`OCTARQ_TRUST_PROXY` when clients can reach octarq directly.

## Notes

- octarq reads the `.mmdb` into memory at startup (not mmap) so it works on overlay
  / network volumes that don't support mmap. A database swapped on disk is picked
  up on the next restart.
- Only **GeoLite2-City** is needed. The Country edition also works but yields no
  region/city.
- A missing or invalid `OCTARQ_GEOIP_DB` file stops startup with an error. Remove
  the variable to run without geo data.
