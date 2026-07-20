# ChatBridge

A bridge application that connects Chatwoot (a customer relationship platform) with WhatsApp using wwebjs (WhatsApp Web JS API wrapper) to allow agents to chat with customers.

## Language

**Session**:
A connection mapping between a Chatwoot inbox/account pair and a WhatsApp instance.
_Avoid_: Connection, link

**Chat**:
A mapping between a Chatwoot conversation and a WhatsApp chat (contact or group).
_Avoid_: Conversation, discussion

**Read Receipt**:
A status indicator sent to WhatsApp to mark messages as read, resulting in blue checkmarks (checklist biru) for the customer.
_Avoid_: Seen receipt, blue tick

**Unread Count**:
The number of messages in a Chatwoot conversation that have not been read by an agent.
_Avoid_: New messages count

**Catch-up Sync**:
Proses pengambilan dan penerusan pesan masuk yang terlewat selama sesi WhatsApp terputus atau terlogout, yang dipicu otomatis begitu status sesi kembali menjadi Ready.
_Avoid_: Sinkronisasi riwayat, sinkronisasi penuh, replay

