# Real-Time Chat Implementation - Debugging Guide

## What Was Fixed

1. **WebSocket Hub Broadcasting**: Fixed the `broadcastToGroup` function to properly use the hub's broadcast channel
2. **Added Debug Logging**: Added comprehensive logging at every step of the WebSocket flow
3. **URL Construction**: Fixed WebSocket URL to use correct path (removed double `/api`)

## How to Test

1. **Start the server** and watch for these logs:
   - `🔧 Initializing WebSocket Hub...`
   - `✅ WebSocket Hub initialized successfully`

2. **Open two devices** and join the same group chat

3. **Watch server logs** when a message is sent:
   ```
   📤 Broadcasting message {ID} to group {groupId}
   📬 Hub received broadcast for group {groupId}, type: message
   📨 Broadcasting to {N} clients in group {groupId}
   ✅ Message sent to user {userID}
   ```

4. **Watch client logs** when receiving messages:
   ```
   🔌 Connecting to WebSocket: ws://...
   ✅ WebSocket connected for group {groupId}
   📩 WebSocket message received: {...}
   📥 Processing WebSocket message: {...}
   🔄 Refetching messages due to new message
   ```

## Debugging Steps

If real-time still doesn't work:

1. **Check if WebSocket connects**: Look for "✅ WebSocket connected" in client logs
2. **Check if messages are broadcast**: Look for "📬 Hub received broadcast" in server logs  
3. **Check if messages are sent**: Look for "✅ Message sent to user" in server logs
4. **Check if client receives**: Look for "📩 WebSocket message received" in client logs

## Common Issues

### Issue: No clients connected
**Server log**: `⚠️ No clients connected for group {groupId}`
**Fix**: The WebSocket connection failed. Check:
- Token authentication
- Network connectivity
- WebSocket URL is correct

### Issue: WebSocket not connecting
**Client log**: No "✅ WebSocket connected" message
**Fix**: Check:
- Server is running and WebSocket route is registered
- Token is valid
- Network connectivity

### Issue: Messages sent but not received
**Symptoms**: Server shows messages sent but client doesn't receive
**Fix**: Check that the client's `onmessage` handler is working

## Files Modified

- `apartmentscloneserver/websocket/hub.go` - Fixed broadcast flow
- `apartmentscloneserver/websocket/handler.go` - Added logging
- `apartmentsclone/hooks/useWebSocket.ts` - Fixed URL construction, added logging  
- `apartmentsclone/screens/GroupChatScreen.tsx` - Added message processing logs

