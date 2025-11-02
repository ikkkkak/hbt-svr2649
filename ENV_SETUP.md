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

