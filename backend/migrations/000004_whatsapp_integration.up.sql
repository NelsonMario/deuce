-- One public WhatsApp group receives one durable announcement per match.
-- Authentication for the bot account itself is stored by whatsmeow in its
-- separate `whatsmeow` schema, not in Deuce domain tables.
CREATE TABLE whatsapp_group_outbox (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    match_id        uuid NOT NULL UNIQUE REFERENCES matches(id) ON DELETE CASCADE,
    group_jid       text NOT NULL,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING', 'SENDING', 'SENT', 'FAILED', 'CANCELLED')),
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error      text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_whatsapp_group_outbox_pending
    ON whatsapp_group_outbox (next_attempt_at, created_at)
    WHERE status = 'PENDING';
