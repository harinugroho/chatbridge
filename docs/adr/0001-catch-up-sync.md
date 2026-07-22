# 0001-catch-up-sync

## Status
accepted

## Context & Decision
We need to synchronize messages that were sent or received while a WhatsApp session was offline/logged out (Catch-up Sync), triggered automatically when the session becomes `ready`. We decided to introduce a new `last_active_at` timestamp column to the `sessions` table which is continuously updated when the session is active. Upon receiving the `ready` event:
1. We query all chats using `/client/getChats`.
2. We fetch the last 100 messages using `/chat/fetchMessages` for chats active since `last_active_at`.
3. We filter and forward new incoming and outgoing messages to Chatwoot.
4. We enforce database-level deduplication via a `UNIQUE` constraint on `messages.whatsapp_id`.
5. We skip sending outgoing messages back to WhatsApp by checking if their Chatwoot message ID is already mapped in the database.
