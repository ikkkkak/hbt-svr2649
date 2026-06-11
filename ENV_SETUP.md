# Environment Variables Setup Guide

## FCM (Firebase Cloud Messaging) Configuration

### Server Environment Variables (.env file)

Add these to **`apartmentscloneserver/.env`**:

```env
# FCM Service Account Credentials Path
# Option 1: Use FCM_CREDENTIALS_PATH (recommended)
FCM_CREDENTIALS_PATH=./service-account.json

# Option 2: Or use GOOGLE_APPLICATION_CREDENTIALS (standard Google Cloud variable)
# GOOGLE_APPLICATION_CREDENTIALS=./service-account.json
```

### Important Notes:

1. **Frontend vs Backend Files:**
   - **Frontend** (`apartmentsclone/google-services.json`): Client-side Firebase config for React Native/Expo ✅ Already in place
   - **Backend** (`apartmentscloneserver/service-account.json`): Server-side Firebase Admin SDK credentials ❌ Need to download this

2. **Two Different Files:**
   - `google-services.json` (frontend): Used by your React Native app
   - `service-account.json` (backend): Used by the Go server to send push notifications

3. **Where to Get Service Account JSON:**
   - Go to [Firebase Console](https://console.firebase.google.com/)
   - Select your project
   - Go to **Project Settings** → **Service Accounts**
   - Click **"Generate new private key"**
   - Save the downloaded JSON file as `service-account.json` in `apartmentscloneserver/`

4. **Automatic Fallback:**
   If the environment variable is not set, the server will automatically look for these files in order:
   - `./service-account.json`
   - `./google-services.json`
   - `./fcm-credentials.json`
   - `../service-account.json`
   - `../google-services.json`

5. **Security:**
   - The `.gitignore` file already excludes `.env` and `service-account.json`
   - Never commit these files to Git
   - On production (Render/other platforms), set the environment variable in the platform's dashboard

### Example .env File Structure:

```env
# Database
DATABASE_URL=postgresql://...

# JWT Secrets
ACCESS_TOKEN_SECRET=...
REFRESH_TOKEN_SECRET=...
EMAIL_TOKEN_SECRET=...

# Redis
REDIS_URL=redis://...

# AWS S3
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
AWS_REGION=...
AWS_BUCKET_NAME=...

# FCM (Firebase Cloud Messaging)
FCM_CREDENTIALS_PATH=./service-account.json
```

### Verification:

After setting up, restart your server and check logs for:
- ✅ `FCM initialized successfully` - FCM is working
- ⚠️ `FCM initialization failed` - Check the credentials file path and format

---

## Google Cloud Run (required for login & profile to work)

**Why 401 and “profile creation / login failing” on Cloud Run?**

On Cloud Run the server runs in a Docker container. There is **no `.env` file** in the container — all variables must be set in the **Cloud Run service** (Console → your service → Edit & deploy new revision → Variables & Secrets).

If `ACCESS_TOKEN_SECRET` or `REFRESH_TOKEN_SECRET` are **not set** (or empty) on Cloud Run:

- JWT verification fails → every protected request returns **401 Unauthorized**
- Profile and login appear to “fail” (USER DATA null, 401 on `/apartment/host/reservations`, etc.)

**Required env vars on Cloud Run (set in Service → Edit → Variables & Secrets):**

| Variable | Required | Notes |
|----------|----------|--------|
| `DB_CONNECTION_STRING` | ✅ Yes | e.g. Cloud SQL connection string |
| `ACCESS_TOKEN_SECRET` | ✅ Yes | **Must match** the value in your local `.env` (same secret for signing/verifying JWTs) |
| `REFRESH_TOKEN_SECRET` | ✅ Yes | **Must match** local `.env`; if missing/empty, refresh and auth break |
| `PORT` | ✅ Yes | Cloud Run sets this (e.g. 8080); only override if needed |

**Steps:**

1. In **Google Cloud Console** → **Cloud Run** → your service (e.g. `habitat-server`) → **Edit & deploy new revision**.
2. Open **Variables & Secrets**.
3. Add (or update):
   - `ACCESS_TOKEN_SECRET` = same value as in your local `.env`
   - `REFRESH_TOKEN_SECRET` = same value as in your local `.env` (if it’s empty locally, generate a strong secret and set it both locally and here)
4. Redeploy. The server will **fail to start** with a clear error if either secret is still missing.

**Local `.env`:** Ensure `REFRESH_TOKEN_SECRET` is set (not empty). Example:

```env
ACCESS_TOKEN_SECRET=your-long-random-secret-here
REFRESH_TOKEN_SECRET=another-long-random-secret-here
```

Use the **same** values on Cloud Run so tokens issued in production are valid.

