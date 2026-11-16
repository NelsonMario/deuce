-- WhatsApp bot settings live in the database so admins configure the bot
-- from WhatsApp (`setup admin` / `setup announce`) instead of editing .env
-- and restarting. Exactly one row (id = 1).
CREATE TABLE whatsapp_bot_config (
    id                 smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    club_id            uuid REFERENCES clubs(id) ON DELETE SET NULL,
    -- Group where admins type commands. Every member of it is an admin.
    admin_group_jid    text,
    -- Group that only receives announcements (lineups, results).
    announce_group_jid text,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- Host-assigned skill grade (A strongest .. E). It sets a new member's
-- starting rating; afterwards the rating moves with match results and the
-- grade stays as the host's reference for session recaps.
ALTER TABLE club_members
    ADD COLUMN grade char(1) CHECK (grade IN ('A', 'B', 'C', 'D', 'E'));
