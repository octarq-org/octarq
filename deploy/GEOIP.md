# GeoIP (GeoLite2) setup

led resolves a click's IP into country / region / city for the analytics
breakdowns. This is **optional** — without a database, geo columns are simply
empty and everything else works. The lookup uses MaxMind's free **GeoLite2-City**
database (an `.mmdb` file).

led doesn't ship the database (licensing terms + ~60 MB), so you bring your own.
Point led at it with **`LED_GEOIP_DB`** (path to the `.mmdb`); an empty/unset value
disables geo lookups.

## 1. Get GeoLite2-City.mmdb

You have two ways to obtain the file. The community mirrors need **no account and
no license key** and are the easiest; MaxMind's official download is the freshest
and most license-clean but requires a free key.

### Option A — community GitHub mirror (no key, easiest)

Several projects republish the free GeoLite2 databases, auto-updated by GitHub
Actions on MaxMind's own Tuesday/Friday schedule. No signup:

```bash
# jsDelivr CDN (gzipped — gunzip after download):
curl -L https://cdn.jsdelivr.net/npm/geolite2-city/GeoLite2-City.mmdb.gz \
  | gunzip > GeoLite2-City.mmdb

# or the P3TERX release mirror (already an .mmdb, no gunzip):
curl -Lo GeoLite2-City.mmdb \
  https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb
```

These mirrors redistribute MaxMind's data under its license (GeoLite2 is offered
under CC BY-SA 4.0); using them is fine — that restriction is just why **led**
itself doesn't bundle the file. If you'd rather not depend on a third party, or
want guaranteed freshness, use Option B.

### Option B — official MaxMind download (free key)

1. Create a free account at <https://www.maxmind.com/en/geolite2/signup>.
2. In **Account → Manage License Keys**, generate a license key.
3. Download and extract the City database:

   ```bash
   KEY=your_license_key
   curl -L "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=${KEY}&suffix=tar.gz" \
     -o GeoLite2-City.tar.gz
   tar -xzf GeoLite2-City.tar.gz --strip-components=1 --wildcards '*/GeoLite2-City.mmdb'
   # → GeoLite2-City.mmdb in the current directory
   ```

   The official [`geoipupdate`](https://github.com/maxmind/geoipupdate) tool
   automates weekly refreshes (it's also the basis of the k8s initContainer
   option below).

## 2. Wire it into your deployment

### Bare binary / local

```bash
LED_GEOIP_DB=/path/to/GeoLite2-City.mmdb ./led
```

### Docker Compose — bind mount

Keep the file on the host and mount it read-only (see the commented block in
[`docker-compose.yml`](../docker-compose.yml)):

```yaml
environment:
  LED_GEOIP_DB: /geoip/GeoLite2-City.mmdb
volumes:
  - /abs/host/path/GeoLite2-City.mmdb:/geoip/GeoLite2-City.mmdb:ro
```

### Kubernetes — bake it into a private image (recommended)

A host bind mount doesn't translate to k8s, and the database is far too big for a
ConfigMap (1 MB limit). The clean answer is to **bake it into your own private
image** with [`deploy/Dockerfile.geoip`](Dockerfile.geoip):

```bash
docker build -t led:base .                              # base image, no DB (may be public)
docker build -f deploy/Dockerfile.geoip \
  --build-arg BASE=led:base \
  -t registry.example.com/led:geoip .                   # private image WITH the DB
```

That image sets `LED_GEOIP_DB` itself, so the Deployment needs no volume and no
extra env. GeoLite2 is CC BY-SA 4.0, so redistributing the baked image is allowed
**with attribution**; the simplest path is to **keep it in your private registry**
and skip that burden.

### Kubernetes — initContainer download (alternative)

If you'd rather not rebuild the image on each GeoLite2 refresh, download it at pod
start into a shared `emptyDir`, keeping only your MaxMind license key in a Secret:

```yaml
spec:
  volumes:
    - name: geoip
      emptyDir: {}
  initContainers:
    - name: fetch-geoip
      image: ghcr.io/maxmind/geoipupdate:latest
      env:
        - name: GEOIPUPDATE_ACCOUNT_ID
          valueFrom: { secretKeyRef: { name: maxmind, key: accountId } }
        - name: GEOIPUPDATE_LICENSE_KEY
          valueFrom: { secretKeyRef: { name: maxmind, key: licenseKey } }
        - name: GEOIPUPDATE_EDITION_IDS
          value: GeoLite2-City
        - name: GEOIPUPDATE_DB_DIR
          value: /geoip
      volumeMounts:
        - { name: geoip, mountPath: /geoip }
  containers:
    - name: led
      image: registry.example.com/led:base
      env:
        - name: LED_GEOIP_DB
          value: /geoip/GeoLite2-City.mmdb
      volumeMounts:
        - { name: geoip, mountPath: /geoip, readOnly: true }
```

## Notes

- led reads the `.mmdb` into memory at startup (not mmap) so it works on overlay
  / network volumes that don't support mmap. A database swapped on disk is picked
  up on the next restart.
- Only **GeoLite2-City** is needed. The Country edition also works but yields no
  region/city.
