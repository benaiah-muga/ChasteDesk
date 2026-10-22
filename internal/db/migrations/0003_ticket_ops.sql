-- SPDX-License-Identifier: AGPL-3.0-only
CREATE SEQUENCE IF NOT EXISTS conversation_ticket_number_seq START WITH 1001;

ALTER TABLE conversations
  ADD COLUMN ticket_number bigint,
  ADD COLUMN priority text NOT NULL DEFAULT 'normal',
  ADD COLUMN tags text[] NOT NULL DEFAULT '{}',
  ADD COLUMN snoozed_until timestamp with time zone;

ALTER TABLE conversations ALTER COLUMN ticket_number SET DEFAULT nextval('conversation_ticket_number_seq');
UPDATE conversations SET ticket_number = nextval('conversation_ticket_number_seq') WHERE ticket_number IS NULL;
ALTER TABLE conversations ALTER COLUMN ticket_number SET NOT NULL;
ALTER SEQUENCE conversation_ticket_number_seq OWNED BY conversations.ticket_number;

ALTER TABLE conversations
  ADD CONSTRAINT conversations_priority_check CHECK (priority IN ('low', 'normal', 'high', 'urgent'));

CREATE UNIQUE INDEX conversations_ticket_number_uq ON conversations (ticket_number);
CREATE INDEX conversations_tags_gin ON conversations USING gin (tags);
CREATE INDEX conversations_org_priority_idx ON conversations (org_id, priority, updated_at DESC);
CREATE INDEX conversations_snoozed_idx ON conversations (org_id, snoozed_until) WHERE snoozed_until IS NOT NULL;

CREATE TABLE canned_replies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  title text NOT NULL,
  body text NOT NULL,
  created_by text REFERENCES "user"(id) ON DELETE SET NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX canned_replies_org_title_uq ON canned_replies (org_id, lower(title));
CREATE INDEX canned_replies_org_idx ON canned_replies (org_id, title);
