# Access token expiry and refresh (client)

## What was happening

- Access tokens expire (default **15 minutes**). The client kept sending an **expired** token on every request (feed, view, etc.).
- Optional-auth endpoints correctly treated the request as unauthenticated and continued, but the server logged `"jwt: token expired"` on **every** request, causing log spam.

## Server changes

1. **No more log spam**  
   When the access token is **expired**, the server no longer logs. The request still proceeds as unauthenticated.

2. **`X-Token-Expired: true` header**  
   If the server receives an expired access token, it sets:
   ```
   X-Token-Expired: true
   ```
   on the response. The client should use this to trigger a refresh and then retry or update stored tokens.

3. **`expiresIn` in login and refresh**  
   Login and `POST /api/auth/refresh` responses now include:
   ```json
   {
     "accessToken": "...",
     "refreshToken": "...",
     "expiresIn": 900
   }
   ```
   `expiresIn` is the access token lifetime in **seconds** (e.g. 900 = 15 minutes). Use it to refresh **before** expiry (e.g. when 80% of `expiresIn` has passed).

4. **Configurable access token lifetime**  
   - Env: `ACCESS_TOKEN_EXPIRY_MINUTES` (default: 15).  
   - Examples: `60` = 1 hour, `10080` = 7 days.  
   - See `.env.example`.

## Client implementation

### 1. React / axios – response interceptor

When any response has `X-Token-Expired: true`:

1. Call `POST /api/auth/refresh` with `{ "refreshToken": "<stored_refresh_token>" }`.
2. Store the new `accessToken` and `refreshToken` from the response.
3. Retry the **original** request with the new `Authorization: Bearer <new_access_token>` (or at least use the new access token for all following requests).

Example pattern:

```js
// On 200 response, check header
if (response.headers['x-token-expired'] === 'true') {
  const { accessToken, refreshToken } = await api.post('/api/auth/refresh', { refreshToken: getStoredRefreshToken() });
  setStoredTokens(accessToken, refreshToken);
  // Retry original request with new accessToken
  return api.request(originalRequest);
}
```

### 2. Proactive refresh (recommended)

Use `expiresIn` from login/refresh:

- Store `expiresAt = now + expiresIn` (in seconds) when you receive tokens.
- Before each API call (or on a timer), if `now >= expiresAt - 60` (e.g. 1 minute before expiry), call `POST /api/auth/refresh`, update stored tokens and `expiresAt`.

This avoids most `X-Token-Expired` responses.

### 3. Optional-auth vs required-auth

- **Optional-auth** (feed, view, search, etc.): if the token is expired, the server continues as **unauthenticated** and sets `X-Token-Expired: true`. The request does **not** return 401.
- **Required-auth** (e.g. `accessTokenVerifierMiddleware`): an expired token typically returns **401**. The client should then refresh and retry.

For optional-auth, the main benefit of reacting to `X-Token-Expired` is to refresh in the background and restore “logged in” behaviour on the next requests, without forcing a 401.

## “Never expires” and security

- Access tokens are **short-lived by design**. Making them very long (e.g. 7 days via `ACCESS_TOKEN_EXPIRY_MINUTES=10080`) reduces refresh frequency but increases risk if a token is stolen.
- Prefer: **short-lived access tokens** (e.g. 15–60 minutes) + **proactive or on-demand refresh** using the long-lived refresh token.
