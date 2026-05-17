# Chatwoot ↔ WhatsApp Web Bridge

An elegant, highly robust orchestration bridge connecting **Chatwoot** (an open-source customer engagement suite) with **WhatsApp** via the [wwebjs-api](https://github.com/orkestral/wwebjs-api) client. 

This bridge enables your support agents to receive, reply to, and manage WhatsApp messages directly from the Chatwoot dashboard, supporting rich media attachments, replies/threading, group chat name resolution, and real-time typing indicators.

---

## 🏗️ Architecture Flow

```mermaid
sequenceDiagram
    autonumber
    actor Customer as WhatsApp User
    participant WA as WhatsApp API (wwebjs)
    participant Bridge as Chatbridge App (Go)
    participant CW as Chatwoot Server
    actor Agent as Chatwoot Agent

    %% Incoming flow
    Customer->>WA: Sends message (Text/Media)
    WA->>Bridge: Webhook: message_create
    Note over Bridge: Resolves contact details<br/>& threads history
    Bridge->>CW: API: Create Contact & Conversation
    Bridge->>CW: API: Send Message (Incoming)
    CW-->>Agent: Message displayed in Dashboard

    %% Outgoing flow
    Agent->>CW: Types reply
    CW->>Bridge: Webhook: message_created (Outgoing)
    Note over Bridge: Validates session<br/>& parses media URLs
    Bridge->>WA: API: Send Message / Media
    WA-->>Customer: Delivers message to phone
```

---

## ⚙️ How it Works

The bridge operates in a **multi-tenant, session-isolated design**:
* **Sessions** are uniquely mapped to specific Chatwoot `account_id` and `inbox_id` pairs (with the ID format `chatwoot-{account_id}-{inbox_id}`).
* **Authentication credentials** (Chatwoot User Access Token and Inbox Bot Token) are retrieved dynamically from the `sessions` database table, avoiding hardcoded master tokens and allowing multiple WhatsApp numbers on different inboxes simultaneously.
* An **Admin Chat** is used to control, initialize, and display the QR authentication code for WhatsApp.

---

## 🚀 Setup & Configuration Guide

Follow these steps to connect your Chatwoot instance to the WhatsApp bridge.

### Step 1: Create an API Inbox in Chatwoot
1. Log in to your Chatwoot Dashboard as an Administrator.
2. Go to **Settings** → **Inboxes** → **Add Inbox**.
3. Choose the **API Channel** as the inbox type.
4. Input an Inbox Name (e.g., `WhatsApp - Support`) and configure agent access.
5. Once created, Chatwoot will display an **API Inbox Token**. Keep this token handy; this is your **`bot_token`**.

### Step 2: Retrieve your User Access Token
The bridge needs a User Access Token to create contacts and initiate conversations.
1. Click on your profile picture at the bottom left of the Chatwoot dashboard.
2. Click **Profile Settings**.
3. Scroll to the very bottom to find the **Access Token** section.
4. Copy this token; this is your **`user_token`**.

### Step 3: Configure the Chatwoot Webhook
1. Go to **Settings** → **Integrations** → **Webhooks** → **Add Webhook**.
2. Set the Webhook URL to:
   ```http
   http://<your-bridge-host>:8080/webhook/chatwoot
   ```
3. Check the following **Subscription Events**:
   * `message_created` (Required to sync messages)
   * `conversation_typing_on` (Optional: displays typing status to WhatsApp)
   * `conversation_typing_off` (Optional)
4. Click **Create Webhook**.

---

## 📱 WhatsApp Connection & Commands (The Admin Chat)

The bridge utilizes a special **System Phone Number** configured in your environment variables (e.g. `SYSTEM_PHONE_NUMBER=+62111111`) to manage your WhatsApp sessions directly inside the Chatwoot dashboard.

### Step 4: Create the Admin Chat
1. In Chatwoot, go to your **Contacts** list and click **New Contact**.
2. Set the Name to `WhatsApp Admin` and set the Phone Number to the exact value of your `SYSTEM_PHONE_NUMBER` (including the `+` prefix, e.g. `+62111111`).
3. Click **Save Contact**.
4. Start a new conversation with this contact in the **API Inbox** you created in Step 1.

### Step 5: Initialize and Connect WhatsApp
Inside the new `WhatsApp Admin` conversation, type and send the initialization command containing your tokens:

```text
init-<user_token>-<bot_token>
```

> [!NOTE]
> Replace `<user_token>` and `<bot_token>` with the tokens you retrieved in Step 1 and Step 2.

**Example Command:**
```text
init-xyz123abc456-bot_token_7890123
```

#### What happens next:
1. The bridge registers the tokens in the PostgreSQL database.
2. The bridge contacts the `wwebjs-api` container to initiate a WhatsApp Web browser instance.
3. The bridge retrieves the generated WhatsApp login **QR Code** and automatically uploads it as an image attachment right inside your Admin conversation.
4. Open WhatsApp on your mobile phone, go to **Linked Devices** → **Link a Device**, and scan the QR code.
5. Once scanned, the chat status will update to **Connected** and all incoming/outgoing messages in that inbox will automatically route through WhatsApp!

---

## 🛠️ Admin Control Commands

You can send these commands anytime inside the `WhatsApp Admin` conversation to manage your WhatsApp session:

| Command | Action | Description |
| :--- | :--- | :--- |
| `init-<user_token>-<bot_token>` | **Initialize Session** | Starts the WhatsApp web browser instance, saves tokens, and outputs the login QR code. |
| `ss` | **Get Screenshot** | Captures a live screenshot of the headless browser page. Extremely useful to debug connection delays, phone offline screens, or sync status. |
| `stop` | **Terminate Session** | Gracefully disconnects, stops the headless browser, and logs out the WhatsApp session. |

---

## 🐳 Environment Parameters (`.env`)

These environment variables configure the bridge at the system level:

```bash
# PostgreSQL Database Connection
DATABASE_URL=postgres://chatbridge:chatbridge@postgres:5432/chatbridge?sslmode=disable

# Chatwoot Instance URL (Internal or External)
CHATWOOT_BASE_URL=http://chatwoot-server:3000

# WhatsApp API URL (points to wwebjs-api container)
WHATSAPP_BASE_URL=http://wwebjs:3000

# WhatsApp API Authorization Token (if API_KEY is enabled on wwebjs)
WHATSAPP_TOKEN=your-secret-token

# Server Port
LISTEN_PORT=8080

# The admin chat key (must match the contact created in Chatwoot)
SYSTEM_PHONE_NUMBER=+62111111
```
