import { readable } from 'svelte/store';
import { createSessionStore } from './session.js';
import { createDataStore } from './storage.js';
import { env } from '../../generated/env.js';
import { makeUrl } from './url_checker.js';
import adminMenu from './admin_menu.svelte';

export const repoUrl = makeUrl(env.cms.repo);
const local = env.local;
const provider = env.cms.provider.toLowerCase();

let authorization_endpoint, access_token_endpoint;
if (!provider || provider === "gitlab") {
    authorization_endpoint = "/oauth/authorize";
    access_token_endpoint = "/oauth/token";
}
if (provider === "gitea" || provider === "forgejo") {
    authorization_endpoint = "/login/oauth/authorize";
    access_token_endpoint = "/login/oauth/access_token";
}

// Optional endpoint overrides (env.cms.authorizationUrl / accessTokenUrl). When an
// override is absent (empty string), fall back to the original derivation —
// repoUrl.origin + the provider's default path — so existing sites are unchanged.
// This lets the OAuth token exchange be routed through a same-origin proxy (e.g. when
// the editor runs on a custom domain) while OAuth authorization can stay a top-level
// navigation to the git host's canonical origin.
const authorizationUrl = env.cms.authorizationUrl || repoUrl.origin + authorization_endpoint;
const accessTokenUrl = env.cms.accessTokenUrl || repoUrl.origin + access_token_endpoint;

const settings = {
    provider: provider,
    authorization_endpoint: authorization_endpoint,
    access_token_endpoint: access_token_endpoint,
    // Fully-resolved, override-aware endpoint URLs (used in place of `server + endpoint`).
    authorizationUrl: authorizationUrl,
    accessTokenUrl: accessTokenUrl,
    server: repoUrl.origin,
    group: repoUrl.pathname.split('/')[1],
    repository: repoUrl.pathname.split('/')[2],
    redirectUrl: env.cms.redirectUrl,
    appId: env.cms.appId
};

const localTokenStore = createDataStore('local_tokens');
let localTokens;
localTokenStore.subscribe(value => {
    localTokens = value;
});
const tokenStore = createDataStore('tokens');
let tokens, isExpired;
tokenStore.subscribe(value => {
    tokens = value;
    isExpired = tokens && Date.now() > (tokens.created_at + tokens.expires_in) * 1000;
});

const codeVerifierStore = createDataStore('code_verifier');
let codeVerifier;
codeVerifierStore.subscribe(value => codeVerifier = value);

const stateStore = createSessionStore('state');
let state;
stateStore.subscribe(value => state = value);

const getUser = () => ({
    isBeingAuthenticated: Boolean(state) || (tokens && isExpired),
    isAuthenticated: localTokens || (tokens && !isExpired),
    tokens,

    finishAuthentication(params) {
        if (params && state && params.get('state') === state) {
            stateStore.set(null);
            history.replaceState(null, '', location.pathname);
            return requestAccessToken(params.get('code'));
        }

        if (tokens && isExpired) {
            return requestRefreshToken();
        }

        console.error('Invalid parameters or state');
    },

    refresh() {
        let authTokens = JSON.parse(localStorage.getItem('PLENTI_CMS_TOKENS'));
        this.isAuthenticated = typeof authTokens?.access_token !== 'undefined';
        this.tokens = authTokens;
    },

    login() {
        return requestAuthCode();
    },

    logout() {
        if (local) {
            localTokenStore.set(null);
            return;
        }
        tokenStore.set(null);
        codeVerifierStore.set(null);
    },

    menu: adminMenu,

});
export const user = readable(getUser(), set => {
    localTokenStore.subscribe(() => set(getUser()));
    tokenStore.subscribe(() => set(getUser()));
    stateStore.subscribe(() => set(getUser()));
});

const generateString = () => {
    const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-.';
    const randomValues = Array.from(crypto.getRandomValues(new Uint8Array(128)));
    return randomValues.map(val => chars[val % chars.length]).join('');
};

const hash = async text => {
    const encoder = new TextEncoder();
    const data = encoder.encode(text);
    const digest = await crypto.subtle.digest('SHA-256', data);
    const binary = String.fromCharCode(...new Uint8Array(digest));
    return btoa(binary)
        .split('=')[0]
        .replace(/\+/g, '-')
        .replace(/\//g, '_');
};

const requestAuthCode = async () => {
    if (local) {
        localTokenStore.set(true);
        return;
    }
    stateStore.set(generateString());
    codeVerifierStore.set(generateString());
    const codeChallenge = await hash(codeVerifier);

    const { authorizationUrl, redirectUrl, appId } = settings;
    window.location.href = authorizationUrl
        + "?client_id=" + encodeURIComponent(appId)
        + "&redirect_uri=" + encodeURIComponent(redirectUrl)
        + "&response_type=code"
        + "&state=" + encodeURIComponent(state)
        + "&code_challenge=" + encodeURIComponent(codeChallenge)
        + "&code_challenge_method=S256";
};

const requestAccessToken = async code => {
    const { accessTokenUrl, redirectUrl, appId } = settings;
    const response = await fetch(accessTokenUrl
        + "?client_id=" + encodeURIComponent(appId)
        + "&code=" + encodeURIComponent(code)
        + "&grant_type=authorization_code"
        + "&redirect_uri=" + encodeURIComponent(redirectUrl)
        + "&code_verifier=" + encodeURIComponent(codeVerifier),
        {
            method: 'POST',
            headers: {
                "Content-Type": "application/x-www-form-urlencoded"
            }
        }
    );
    const tokens = await response.json();
    if (tokens.error) {
        throw new Error(tokens.error_description);
    }
    tokenStore.set(tokens);
};

const requestRefreshToken = async () => {
    const { accessTokenUrl, redirectUrl, appId } = settings;
    if (!codeVerifier) {
        throw new Error("Code verifier not saved to session storage");
    }
    // Same override-aware token endpoint as the initial code exchange, so a proxied
    // token URL applies to refresh too (otherwise refresh would go cross-origin).
    const response = await fetch(accessTokenUrl
        + "?client_id=" + encodeURIComponent(appId)
        + "&refresh_token=" + encodeURIComponent(tokens.refresh_token)
        + "&grant_type=refresh_token"
        + "&redirect_uri=" + encodeURIComponent(redirectUrl)
        + "&code_verifier=" + encodeURIComponent(codeVerifier),
        {
            method: 'POST',
            headers: {
                "Content-Type": "application/x-www-form-urlencoded"
            }
        }
    );
    const refreshedTokens = await response.json();
    if (refreshedTokens.error) {
        throw new Error(refreshedTokens.error_description);
    }
    tokenStore.set(refreshedTokens);
};