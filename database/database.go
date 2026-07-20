package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// DB wraps the sql.DB connection and provides query methods.
type DB struct {
	*sql.DB
}

// New creates a new database connection and runs migrations with retry logic.
func New(databaseURL string) (*DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection with retries
	var lastErr error
	maxAttempts := 15
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		lastErr = err
		log.Printf("[DB] Database not ready yet (attempt %d/%d): %v. Retrying in 2 seconds...", attempt, maxAttempts, err)
		time.Sleep(2 * time.Second)
	}

	if lastErr != nil && err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database after %d attempts: %w", maxAttempts, lastErr)
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("[DB] Connected and migrated successfully")
	return d, nil
}

// migrate creates tables if they don't exist.
func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id SERIAL PRIMARY KEY,
			session_id VARCHAR(100) UNIQUE NOT NULL,
			qr TEXT,
			state VARCHAR(50),
			is_contact_sync BOOLEAN DEFAULT FALSE,
			bot_token VARCHAR(255),
			user_token VARCHAR(255),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id SERIAL PRIMARY KEY,
			whatsapp_id VARCHAR(100) UNIQUE,
			whatsapp_lid VARCHAR(100),
			name VARCHAR(255),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS chats (
			id SERIAL PRIMARY KEY,
			whatsapp_id VARCHAR(100) NOT NULL,
			account_id INTEGER NOT NULL,
			inbox_id INTEGER NOT NULL,
			conversation_id INTEGER,
			contact_id INTEGER,
			session_id VARCHAR(100) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(whatsapp_id, session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			chatwoot_id INTEGER NOT NULL,
			whatsapp_id VARCHAR(100) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		// Indexes for common lookups
		`CREATE INDEX IF NOT EXISTS idx_chats_conversation_id ON chats(conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_session_whatsapp ON chats(session_id, whatsapp_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_whatsapp_id ON messages(whatsapp_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chatwoot_id ON messages(chatwoot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_whatsapp_lid ON contacts(whatsapp_lid)`,
	}

	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			return fmt.Errorf("migration query failed: %w\nQuery: %s", err, q)
		}
	}
	return nil
}

// ============================================================
// Session Queries
// ============================================================

// GetSessionBySessionID retrieves a session by its session_id.
func (d *DB) GetSessionBySessionID(ctx context.Context, sessionID string) (*Session, error) {
	s := &Session{}
	err := d.QueryRowContext(ctx,
		`SELECT id, session_id, qr, state, is_contact_sync, bot_token, user_token, created_at, updated_at
		 FROM sessions WHERE session_id = $1`, sessionID,
	).Scan(&s.ID, &s.SessionID, &s.QR, &s.State, &s.IsContactSync, &s.BotToken, &s.UserToken, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpsertSession inserts or updates a session.
func (d *DB) UpsertSession(ctx context.Context, sessionID string, updates map[string]interface{}) error {
	// Build dynamic upsert
	s, err := d.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}

	if s == nil {
		// Insert
		_, err = d.ExecContext(ctx,
			`INSERT INTO sessions (session_id, qr, state, bot_token, user_token)
			 VALUES ($1, $2, $3, $4, $5)`,
			sessionID,
			updates["qr"],
			updates["state"],
			updates["bot_token"],
			updates["user_token"],
		)
	} else {
		// Update only provided fields
		if v, ok := updates["qr"]; ok {
			if _, err := d.ExecContext(ctx, `UPDATE sessions SET qr = $1, updated_at = NOW() WHERE session_id = $2`, v, sessionID); err != nil {
				return err
			}
		}
		if v, ok := updates["state"]; ok {
			if _, err := d.ExecContext(ctx, `UPDATE sessions SET state = $1, updated_at = NOW() WHERE session_id = $2`, v, sessionID); err != nil {
				return err
			}
		}
		if v, ok := updates["bot_token"]; ok {
			if _, err := d.ExecContext(ctx, `UPDATE sessions SET bot_token = $1, updated_at = NOW() WHERE session_id = $2`, v, sessionID); err != nil {
				return err
			}
		}
		if v, ok := updates["user_token"]; ok {
			if _, err := d.ExecContext(ctx, `UPDATE sessions SET user_token = $1, updated_at = NOW() WHERE session_id = $2`, v, sessionID); err != nil {
				return err
			}
		}
	}
	return err
}

// ============================================================
// Contact Queries
// ============================================================

// GetContactByLID retrieves a contact by their WhatsApp LID.
func (d *DB) GetContactByLID(ctx context.Context, lid string) (*Contact, error) {
	c := &Contact{}
	err := d.QueryRowContext(ctx,
		`SELECT id, whatsapp_id, whatsapp_lid, name, created_at, updated_at
		 FROM contacts WHERE whatsapp_lid = $1`, lid,
	).Scan(&c.ID, &c.WhatsAppID, &c.WhatsAppLID, &c.Name, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// UpsertContact inserts or updates a contact by whatsapp_id.
func (d *DB) UpsertContact(ctx context.Context, whatsappID, lid, name string) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO contacts (whatsapp_id, whatsapp_lid, name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (whatsapp_id) DO UPDATE SET
		   whatsapp_lid = COALESCE(EXCLUDED.whatsapp_lid, contacts.whatsapp_lid),
		   name = COALESCE(EXCLUDED.name, contacts.name),
		   updated_at = NOW()`,
		whatsappID, nilIfEmpty(lid), nilIfEmpty(name),
	)
	return err
}

// ============================================================
// Chat Queries
// ============================================================

// GetChatByConversationID retrieves a chat by Chatwoot conversation ID and session ID.
// Both fields are required to avoid cross-account collisions, since conversation IDs
// are only unique per Chatwoot account, not globally.
func (d *DB) GetChatByConversationID(ctx context.Context, conversationID int, sessionID string) (*Chat, error) {
	c := &Chat{}
	err := d.QueryRowContext(ctx,
		`SELECT id, whatsapp_id, account_id, inbox_id, conversation_id, contact_id, session_id, created_at, updated_at
		 FROM chats WHERE conversation_id = $1 AND session_id = $2 LIMIT 1`, conversationID, sessionID,
	).Scan(&c.ID, &c.WhatsAppID, &c.AccountID, &c.InboxID, &c.ConversationID, &c.ContactID, &c.SessionID, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// GetChatByConversationIDOnly retrieves a chat by Chatwoot conversation ID only.
func (d *DB) GetChatByConversationIDOnly(ctx context.Context, conversationID int) (*Chat, error) {
	c := &Chat{}
	err := d.QueryRowContext(ctx,
		`SELECT id, whatsapp_id, account_id, inbox_id, conversation_id, contact_id, session_id, created_at, updated_at
		 FROM chats WHERE conversation_id = $1 LIMIT 1`, conversationID,
	).Scan(&c.ID, &c.WhatsAppID, &c.AccountID, &c.InboxID, &c.ConversationID, &c.ContactID, &c.SessionID, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}


// GetChatByWhatsAppIDAndSession retrieves a chat by WhatsApp ID and session.
func (d *DB) GetChatByWhatsAppIDAndSession(ctx context.Context, whatsappID, sessionID string) (*Chat, error) {
	c := &Chat{}
	err := d.QueryRowContext(ctx,
		`SELECT id, whatsapp_id, account_id, inbox_id, conversation_id, contact_id, session_id, created_at, updated_at
		 FROM chats WHERE whatsapp_id = $1 AND session_id = $2 LIMIT 1`, whatsappID, sessionID,
	).Scan(&c.ID, &c.WhatsAppID, &c.AccountID, &c.InboxID, &c.ConversationID, &c.ContactID, &c.SessionID, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// UpsertChat inserts or updates a chat entry.
func (d *DB) UpsertChat(ctx context.Context, whatsappID string, accountID, inboxID int, conversationID, contactID *int, sessionID string) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO chats (whatsapp_id, account_id, inbox_id, conversation_id, contact_id, session_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (whatsapp_id, session_id) DO UPDATE SET
		   account_id = EXCLUDED.account_id,
		   inbox_id = EXCLUDED.inbox_id,
		   conversation_id = COALESCE(EXCLUDED.conversation_id, chats.conversation_id),
		   contact_id = COALESCE(EXCLUDED.contact_id, chats.contact_id),
		   updated_at = NOW()`,
		whatsappID, accountID, inboxID, conversationID, contactID, sessionID,
	)
	return err
}

// ResetChatConversationID sets the conversation_id to NULL for a given chat.
func (d *DB) ResetChatConversationID(ctx context.Context, sessionID string, conversationID int) error {
	_, err := d.ExecContext(ctx,
		`UPDATE chats SET conversation_id = NULL, updated_at = NOW()
		 WHERE session_id = $1 AND conversation_id = $2`,
		sessionID, conversationID,
	)
	return err
}

// DeleteChatByContactAndSession deletes chat entries matching contact_id and session_id.
func (d *DB) DeleteChatByContactAndSession(ctx context.Context, contactID int, sessionID string) error {
	_, err := d.ExecContext(ctx,
		`DELETE FROM chats WHERE contact_id = $1 AND session_id = $2`,
		contactID, sessionID,
	)
	return err
}

// ============================================================
// Message Queries
// ============================================================

// InsertMessage creates a new message mapping.
func (d *DB) InsertMessage(ctx context.Context, chatwootID int, whatsappID string) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO messages (chatwoot_id, whatsapp_id) VALUES ($1, $2)`,
		chatwootID, whatsappID,
	)
	return err
}

// GetMessageByWhatsAppID retrieves a message mapping by WhatsApp message ID.
func (d *DB) GetMessageByWhatsAppID(ctx context.Context, whatsappID string) (*Message, error) {
	m := &Message{}
	err := d.QueryRowContext(ctx,
		`SELECT id, chatwoot_id, whatsapp_id, created_at, updated_at
		 FROM messages WHERE whatsapp_id = $1 LIMIT 1`, whatsappID,
	).Scan(&m.ID, &m.ChatwootID, &m.WhatsAppID, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// GetMessageByChatwootID retrieves a message mapping by Chatwoot message ID.
func (d *DB) GetMessageByChatwootID(ctx context.Context, chatwootID int) (*Message, error) {
	m := &Message{}
	err := d.QueryRowContext(ctx,
		`SELECT id, chatwoot_id, whatsapp_id, created_at, updated_at
		 FROM messages WHERE chatwoot_id = $1 LIMIT 1`, chatwootID,
	).Scan(&m.ID, &m.ChatwootID, &m.WhatsAppID, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// ============================================================
// Helpers
// ============================================================

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
