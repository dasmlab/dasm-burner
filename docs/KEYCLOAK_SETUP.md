# Keycloak SSO setup for dasm-burner

Same dasmlab realm pattern as mock-me / interview-me.

## Cluster IdP

| Item | Value |
|------|--------|
| Keycloak URL | `https://keycloak.apps.2026-prod-1.ocp.dasmlab.org` |
| Realm | `dasmlab` |
| Issuer | `https://keycloak.apps.2026-prod-1.ocp.dasmlab.org/realms/dasmlab` |
| App URL | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` |
| Client ID | `dasm-burner` |

Admin console: https://keycloak.apps.2026-prod-1.ocp.dasmlab.org/admin

---

## Create client

1. Client type: OpenID Connect · Client ID: `dasm-burner`
2. Client authentication: **ON** (confidential — secret stays on the Go backend)
3. Standard flow: ON · Direct access grants: OFF · Implicit: OFF
4. Login settings:

| Field | Value |
|-------|--------|
| Root URL | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/` |
| Home URL | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/` |
| Valid redirect URIs | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/api/v1/auth/callback` |
| | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/*` |
| | `http://localhost:8080/api/v1/auth/callback` |
| | `http://localhost:8080/*` |
| Valid post logout redirect URIs | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/*` |
| | `http://localhost:8080/*` |
| Web origins | `https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` |
| | `http://localhost:8080` |

5. Client role: **`admin`** — assign to Keycloak user **`dasm`** (Users → dasm → Role mapping → Filter by clients → `dasm-burner` → `admin`)
6. Default client scopes must include **`roles`** so `resource_access["dasm-burner"].roles` lands in the access token. Match is case-insensitive (`Admin` or `admin`).

Without the client role, login succeeds but APIs return 403 and the UI sends you back to login.

---

## How login works

1. User hits the app → redirected to `/login`
2. **Sign in with Keycloak** → `GET /api/v1/auth/login`
3. Keycloak redirects to `/api/v1/auth/callback`
4. Backend exchanges the code with the client secret, verifies `id_token`, sets `db_session` httpOnly cookie
5. SPA calls send the cookie; `AdminMiddleware` requires client role `admin`
6. Logout clears the cookie and hits Keycloak end-session
7. `/healthz`, `/readyz`, `/api/v1/version`, `/api/v1/auth/config` stay public

When OIDC env is unset, serve runs **open local/dev** (no login).

---

## App env vars

```
KEYCLOAK_URL=https://keycloak.apps.2026-prod-1.ocp.dasmlab.org
KEYCLOAK_REALM=dasmlab
OIDC_CLIENT_ID=dasm-burner
OIDC_CLIENT_SECRET=<from Credentials tab — never commit>
APP_PUBLIC_URL=https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org
OIDC_REDIRECT_URI=https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org/api/v1/auth/callback
OIDC_CA_FILE=/etc/oidc/ca.crt
```

(or set `OIDC_ISSUER` directly to the realm issuer URL)

## Prod wiring (2026-prod-1)

- Namespace: `dasm-burner-system`
- Secret: `dasm-burner-oidc` (key `client-secret`) — **do not commit**
- ConfigMap: `dasm-burner-oidc-ca` (lab CA; copy from `mock-me-oidc-ca` in `mock-me-system`)
- Route: https://dasm-burner.apps.2026-prod-1.ocp.dasmlab.org
- Edge cert: HAProxy on `10.20.1.10` (`/home/dasm/dasmlab-internal/new_haproxy/runme.sh`). Same pattern as mock-me CERT55. `scripts/ci/ensure-prod-cert.sh dasm-burner.apps.2026-prod-1.ocp.dasmlab.org` appends `CERTn=` if missing and runs `./runme.sh` (Let's Encrypt + brief proxy recycle). Without that hostname in the CERT list, browsers hit `NET::ERR_CERT_COMMON_NAME_INVALID` and HSTS blocks the bypass.

Create the secret once:

```bash
oc create secret generic dasm-burner-oidc \
  -n dasm-burner-system \
  --from-literal=client-secret='YOUR_CLIENT_SECRET'
```

Copy the CA:

```bash
oc get cm mock-me-oidc-ca -n mock-me-system -o yaml \
  | sed 's/namespace: mock-me-system/namespace: dasm-burner-system/; s/name: mock-me-oidc-ca/name: dasm-burner-oidc-ca/' \
  | oc apply -f -
```
