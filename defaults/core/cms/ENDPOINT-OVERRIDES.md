# CMS endpoint overrides

By default Plenti's Git-CMS derives three browser-facing endpoints from a single origin —
the `repo` URL in `plenti.json`:

| Endpoint | Default (derived from `repo`) |
|---|---|
| OAuth authorization | `<repo-origin>` + provider path (`/login/oauth/authorize` for Gitea/Forgejo, `/oauth/authorize` for GitLab) |
| OAuth token exchange | `<repo-origin>` + provider path (`/login/oauth/access_token` · `/oauth/token`) |
| Provider API base | `<repo-origin>/api/v1` (Gitea/Forgejo) · `<repo-origin>/api/v4` (GitLab) |

This is correct for the common case where the browser reaches the git host directly at one
origin. Some self-hosted / reverse-proxied deployments need these to differ — for example, to
route the **token exchange** and **repository API** through a **same-origin proxy** on the site's
own domain (avoiding CORS) while keeping **OAuth authorization** a top-level navigation to the git
host's canonical origin (so the git host serves its own login/consent UI and its `ROOT_URL` stays
truthful).

## Optional fields

Add any of these to the `cms` block in `plenti.json`. **Each is independent, and each is optional —
when omitted (or empty), the endpoint is derived from `repo` exactly as before, so existing sites
are unaffected.**

| Field | Overrides | Falls back to (when absent) |
|---|---|---|
| `authorization_url` | the full OAuth authorize URL | `<repo-origin>` + provider authorize path |
| `access_token_url` | the full OAuth token URL (used for **both** the code exchange **and** token refresh) | `<repo-origin>` + provider token path |
| `api_base_url` | the provider API base URL | `<repo-origin>/api/v1` or `/api/v4` |

The OAuth flow is unchanged: still a public PKCE client (no client secret), with `state`,
`code_challenge`, `code_challenge_method=S256`, and `code_verifier` exactly as before — only the
endpoint *resolution* changes.

## Example — split origin (token + API proxied, authorize canonical)

```json
{
  "cms": {
    "provider": "gitea",
    "repo": "https://git.example.com/owner/repository",
    "branch": "main",
    "redirect_url": "https://site.example.com",
    "app_id": "public-oauth-client-id",

    "authorization_url": "https://git.example.com/login/oauth/authorize",
    "access_token_url": "https://site.example.com/_cms/git/login/oauth/access_token",
    "api_base_url": "https://site.example.com/_cms/git/api/v1"
  }
}
```

With this config the browser:

- navigates to **`git.example.com`** for authorization (the git host shows its own login/consent);
- exchanges the code and refreshes tokens via **`site.example.com/_cms/git/...`** (same-origin);
- makes repository API calls via **`site.example.com/_cms/git/api/v1`** (same-origin).

Routing `site.example.com/_cms/git/*` → the git host is configured on the site's own web server /
edge (Caddy `reverse_proxy`, Nginx `proxy_pass`, a Netlify/Vercel function, or a Cloudflare
Worker). **That proxy is deployment-specific and lives outside Plenti** — Plenti only needs to be
told which URLs to call. Platforms without a configurable proxy layer (e.g. GitHub Pages, GitLab
Pages) can't use the split; they simply omit these fields and get the default single-origin
behaviour.

## Backward compatibility

Omitting all three fields reproduces the current behaviour exactly. The defaults are unchanged for
GitLab, Gitea, and Forgejo (Forgejo resolves identically to Gitea).
