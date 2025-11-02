# FCM (Firebase Cloud Messaging) Setup Guide

## Overview
The server now supports Firebase Cloud Messaging (FCM) for push notifications. It will automatically fall back to Expo Push Notifications if FCM is not configured.

## Backend Setup

### 1. Create Firebase Project
1. Go to [Firebase Console](https://console.firebase.google.com/)
2. Create a new project or select an existing one
3. Enable Cloud Messaging API

### 2. Generate Service Account Key
1. Go to Project Settings → Service Accounts
2. Click "Generate new private key"
3. Download the JSON file
4. Save it as `service-account.json` in the server root directory

### 3. Configure Environment Variable
Set the path to your service account JSON file:
```bash
export FCM_CREDENTIALS_PATH="./service-account.json"
# OR
export GOOGLE_APPLICATION_CREDENTIALS="./service-account.json"
```

Alternatively, the server will automatically look for:
- `./service-account.json`
- `./google-services.json`
- `./fcm-credentials.json`
- `../service-account.json`
- `../google-services.json`

## Frontend Setup (Already Configured)

The frontend is already configured to use FCM:
- `google-services.json` is referenced in `app.json`
- Token generation uses `getDevicePushTokenAsync()` which returns FCM tokens on Android
- Both FCM and Expo tokens are accepted by the backend

## Token Format

The system accepts both:
- **Expo tokens**: `ExponentPushToken[...]` (~41 characters)
- **FCM tokens**: Long alphanumeric strings (152+ characters)

## Testing

1. Ensure `service-account.json` is in the server root
2. Restart the server
3. Check logs for: `✅ FCM initialized successfully`
4. Register a push token from the app
5. Send a test notification

## Troubleshooting

### FCM Not Initializing
- Check that `service-account.json` exists and is valid
- Verify the service account has "Firebase Cloud Messaging API" enabled
- Check server logs for specific error messages

### Fallback to Expo
If FCM initialization fails, the server automatically falls back to Expo Push Notifications. Check logs for:
```
⚠️ FCM initialization failed: ...
⚠️ Push notifications will fall back to Expo Push service
```

## Files Modified

### Backend
- `services/push/fcm.go` - FCM implementation
- `services/push/service.go` - Unified push service (FCM with Expo fallback)
- `services/push/worker.go` - Updated to use unified service
- `services/push/tokens.go` - Updated to accept both token formats
- `main.go` - FCM initialization
- `routes/messages_broadcast.go` - Updated to use unified service

### Frontend
- `hooks/useNotifications.ts` - Updated to get FCM tokens

## Notes

- FCM tokens work on both Android and iOS
- On iOS, APNs tokens are used (FCM handles them)
- The system automatically removes invalid tokens
- Batch sending is supported for efficiency

