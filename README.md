# Xylon minimal customer interface

Demo **platform admin** product: source of truth for customers (status, features),
lifecycle of shared secrets pushed into
[xylon-simple-secret-store](https://github.com/Lazarus404/xylon-simple-secret-store),
and the **only** lab path that calls Xylon Control for admin (presence, history,
inject, moderate, record, live session list).

Does not mint browser tokens (that is
[xylon-demo-customer-admin-app](https://github.com/Lazarus404/xylon-demo-customer-admin-app)).
SaaS customer backends must call this interface for Control admin -- not Xylon
directly -- so suspended status and feature flags can deny the request.

Store and interface traffic stay on a private network (lab: localhost). There is
**no** ops bearer to the secret store and **no** bearer on this ops API; bind
`127.0.0.1` (or equivalent private reachability) instead.

Related: [GOLANG_APPS.md](../GOLANG_APPS.md),
[XYLON_ADMIN_ARCHITECTURE.md](../XYLON_ADMIN_ARCHITECTURE.md).

## Run

Start the secret store first (`iex -S mix` in `xylon-simple-secret-store`, port
8091). Xylon Control must be enabled (HMAC) on port 8080 for proxy routes.
Then:

```bash
cd xylon-minimal-customer-interface
go run ./cmd/interface -config config.example.json
```

Default listen: `127.0.0.1:8093`.

With `seed_on_start: true`, startup ensures:

- `acme` -- active; Control features on; provisioned into the store if missing
- `broke-co` -- suspended; no store row (deny demo)

## Config

See `config.example.json`:

| Key | Meaning |
| --- | --- |
| `listen_addr` | Bind address (default `127.0.0.1:8093`; **must** be loopback) |
| `store_base_url` | Secret store base URL |
| `xylon_base_url` | Xylon HTTP base for Control proxy (default `http://127.0.0.1:8080`) |
| `control_participant_id` | Participant id embedded in minted Control HMAC (default `platform-admin`) |
| `control_token_ttl_sec` | Mint TTL seconds (default `60`) |
| `admin_config_path` | If set, rewrite demo admin JSON on provision/rotate for `admin_customer_id` |
| `admin_customer_id` | Customer whose secret is written to admin config (default `acme`) |
| `seed_on_start` | Seed demo customers and provision `acme` |
| `default_grace_sec` | Rotate grace when body omits `grace_sec` |
| `webhook_secret` | Same value as Xylon `:control` `webhook_secret` (not tenant HMAC) |
| `admin_webhook_url` | Optional; after verify + ring store, forward identical body + signature |

## HTTP API

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/healthz` | Liveness; includes `store_ok` |
| GET | `/v1/customers` | List (never secrets) |
| GET | `/v1/customers/:id` | One customer + `store_present` |
| PATCH | `/v1/customers/:id` | Optional `status`, `features` merge; re-PUTs store flags when row exists |
| POST | `/v1/customers/:id/provision` | Active + store PUT; returns `hmac_secret` once |
| POST | `/v1/customers/:id/rotate` | Body optional `{ "grace_sec": 900 }`; returns new secret once |
| POST | `/v1/customers/:id/revoke` | Suspended + store DELETE |
| GET | `/v1/customers/:id/sessions` | Gate then proxy live session list |
| GET | `/v1/customers/:id/fleet/summary` | Gate then proxy fleet summary |
| GET | `/v1/customers/:id/fleet/nodes` | Gate then proxy fleet nodes |
| POST | `/v1/customers/:id/fleet/accepting` | Gate then proxy drain/undrain body |
| GET | `/v1/customers/:id/sessions/:sid/presence` | Gate then proxy Control presence |
| GET | `/v1/customers/:id/sessions/:sid/history` | Gate then proxy; query `label` |
| POST | `/v1/customers/:id/sessions/:sid/data` | Gate then proxy inject body |
| POST | `/v1/customers/:id/sessions/:sid/moderate` | Gate then proxy moderate body |
| POST | `/v1/customers/:id/sessions/:sid/record/start` | Gate then proxy record start |
| POST | `/v1/customers/:id/sessions/:sid/record/stop` | Gate then proxy record stop |
| GET | `/v1/customers/:id/sessions/:sid/record` | Gate then proxy record status |
| POST | `/v1/webhooks/xylon` | Signed ingest; ring + optional forward to `admin_webhook_url` |
| GET | `/v1/webhooks/xylon/latest` | Newest webhook event (lab) |

### Control proxy gate

Before calling Xylon:

1. Customer exists (`404` otherwise)
2. `status` is `active` (`403` `customer_suspended`)
3. Features: `control` plus the action flag (`presence` / `history` / `inject` / `moderate` / `record` / `sessions` / `fleet`) must be boolean true (`403` `feature_denied`)
4. Store reveal of `current` secret succeeds

The interface mints a short Control HMAC with the needed capabilities and forwards
Authorization Bearer to Xylon. Record proxies mint caps `["record"]`; session list
mints `["sessions"]`; fleet mints `["fleet"]`. Response status and body are passed through. Xylon still
rejects bearers that list capabilities without the required action (Control ACL).

### Examples

```bash
curl -s http://127.0.0.1:8093/healthz

curl -s http://127.0.0.1:8093/v1/customers

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/provision

curl -s -X PATCH http://127.0.0.1:8093/v1/customers/acme \
  -H 'content-type: application/json' \
  -d '{"features":{"record":false}}'

curl -s http://127.0.0.1:8093/v1/customers/acme/sessions

curl -s http://127.0.0.1:8093/v1/customers/acme/fleet/summary
curl -s http://127.0.0.1:8093/v1/customers/acme/fleet/nodes

curl -s http://127.0.0.1:8093/v1/customers/acme/sessions/room-1/presence

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/sessions/room-1/moderate \
  -H 'content-type: application/json' \
  -d '{"action":"kick","participant_id":"p2"}'

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/sessions/room-1/record/start \
  -H 'content-type: application/json' \
  -d '{"mode":"audio"}'

curl -s http://127.0.0.1:8093/v1/customers/acme/sessions/room-1/record

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/sessions/room-1/record/stop \
  -H 'content-type: application/json' \
  -d '{}'

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/rotate \
  -H 'content-type: application/json' \
  -d '{"grace_sec":900}'

curl -s -X POST http://127.0.0.1:8093/v1/customers/acme/revoke
```

Do not log or paste provision/rotate response secrets into tickets.
