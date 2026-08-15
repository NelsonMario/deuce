-- Fully-automatic mode: let a host opt a session into auto-filling every
-- empty court with no per-court "Generate match" trigger. Defaults to true
-- so existing AUTOMATIC-mode sessions behave the same as before this column
-- existed until a host explicitly turns it off.
ALTER TABLE sessions
    ADD COLUMN auto_fill_enabled boolean NOT NULL DEFAULT true;
