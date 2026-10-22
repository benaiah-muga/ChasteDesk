-- SPDX-License-Identifier: AGPL-3.0-only
ALTER TYPE "channel" ADD VALUE IF NOT EXISTS 'email';

ALTER TABLE conversations
  ADD COLUMN email_subject text,
  ADD COLUMN email_thread_key text;

CREATE UNIQUE INDEX conversations_email_thread_uq
  ON conversations (org_id, email_thread_key)
  WHERE email_thread_key IS NOT NULL;

ALTER TABLE messages ADD COLUMN email_message_id text;

CREATE UNIQUE INDEX messages_email_message_id_uq
  ON messages (org_id, email_message_id)
  WHERE email_message_id IS NOT NULL;

CREATE TABLE email_outbox (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  conversation_id uuid REFERENCES conversations(id) ON DELETE CASCADE,
  source_message_id uuid REFERENCES messages(id) ON DELETE SET NULL,
  kind text NOT NULL CHECK (kind IN ('reply', 'invitation')),
  to_email text NOT NULL,
  subject text NOT NULL,
  text_body text NOT NULL,
  html_body text,
  email_message_id text,
  in_reply_to text,
  references_header text,
  status text DEFAULT 'queued' NOT NULL CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
  attempts integer DEFAULT 0 NOT NULL,
  last_error text,
  next_attempt_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  sent_at timestamp with time zone
);

CREATE UNIQUE INDEX email_outbox_source_message_uq
  ON email_outbox (source_message_id)
  WHERE source_message_id IS NOT NULL;
CREATE INDEX email_outbox_queue_idx ON email_outbox (status, next_attempt_at, created_at);
CREATE INDEX email_outbox_org_idx ON email_outbox (org_id, created_at DESC);

CREATE OR REPLACE FUNCTION enqueue_email_reply() RETURNS trigger AS $$
DECLARE
  conv_channel text;
  conv_org text;
  conv_subject text;
  conv_ticket bigint;
  customer_email text;
  reply_to text;
BEGIN
  IF NEW.role NOT IN ('agent_ai', 'agent_human') THEN
    RETURN NEW;
  END IF;
  SELECT c.channel::text, c.org_id, c.email_subject, c.ticket_number, cu.email
    INTO conv_channel, conv_org, conv_subject, conv_ticket, customer_email
    FROM conversations c JOIN customers cu ON cu.id = c.customer_id
   WHERE c.id = NEW.conversation_id;
  IF conv_channel <> 'email' OR customer_email IS NULL THEN
    RETURN NEW;
  END IF;
  SELECT email_message_id INTO reply_to
    FROM messages
   WHERE conversation_id = NEW.conversation_id AND role = 'customer' AND email_message_id IS NOT NULL
   ORDER BY created_at DESC LIMIT 1;
  INSERT INTO email_outbox (org_id, conversation_id, source_message_id, kind, to_email, subject, text_body, in_reply_to)
  VALUES (
    conv_org, NEW.conversation_id, NEW.id, 'reply', customer_email,
    'Re: ' || COALESCE(NULLIF(conv_subject, ''), 'Support request') || ' [#' || conv_ticket || ']',
    NEW.content, reply_to
  )
  ON CONFLICT (source_message_id) WHERE source_message_id IS NOT NULL DO NOTHING;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER messages_enqueue_email
AFTER INSERT ON messages
FOR EACH ROW EXECUTE FUNCTION enqueue_email_reply();
