-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE sla_policies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  priority text NOT NULL CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
  first_response_minutes integer NOT NULL CHECK (first_response_minutes BETWEEN 1 AND 525600),
  resolution_minutes integer NOT NULL CHECK (resolution_minutes BETWEEN 1 AND 525600),
  enabled boolean DEFAULT true NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX sla_policies_org_priority_uq ON sla_policies (org_id, priority);
CREATE INDEX sla_policies_org_idx ON sla_policies (org_id, priority);

INSERT INTO sla_policies (org_id, priority, first_response_minutes, resolution_minutes)
SELECT o.id, v.priority, v.first_minutes, v.resolution_minutes
FROM organization o
CROSS JOIN (VALUES
  ('urgent', 30, 240),
  ('high', 60, 480),
  ('normal', 240, 1440),
  ('low', 480, 4320)
) AS v(priority, first_minutes, resolution_minutes)
ON CONFLICT (org_id, priority) DO NOTHING;

CREATE OR REPLACE FUNCTION seed_org_sla_policies() RETURNS trigger AS $$
BEGIN
  INSERT INTO sla_policies (org_id, priority, first_response_minutes, resolution_minutes)
  VALUES
    (NEW.id, 'urgent', 30, 240),
    (NEW.id, 'high', 60, 480),
    (NEW.id, 'normal', 240, 1440),
    (NEW.id, 'low', 480, 4320)
  ON CONFLICT (org_id, priority) DO NOTHING;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER organization_seed_sla
AFTER INSERT ON organization
FOR EACH ROW EXECUTE FUNCTION seed_org_sla_policies();

ALTER TABLE conversations
  ADD COLUMN first_response_due_at timestamp with time zone,
  ADD COLUMN resolution_due_at timestamp with time zone;

UPDATE conversations c SET
  first_response_due_at = c.created_at + make_interval(mins => p.first_response_minutes),
  resolution_due_at = c.created_at + make_interval(mins => p.resolution_minutes)
FROM sla_policies p
WHERE p.org_id = c.org_id AND p.priority = c.priority AND p.enabled = true;

CREATE INDEX conversations_sla_idx ON conversations (org_id, resolution_due_at)
WHERE resolution_due_at IS NOT NULL;

CREATE OR REPLACE FUNCTION set_conversation_sla() RETURNS trigger AS $$
DECLARE
  first_minutes integer;
  resolution_minutes integer;
BEGIN
  SELECT p.first_response_minutes, p.resolution_minutes INTO first_minutes, resolution_minutes
  FROM sla_policies p
  WHERE p.org_id = NEW.org_id AND p.priority = NEW.priority AND p.enabled = true;

  IF FOUND THEN
    NEW.first_response_due_at := NEW.created_at + make_interval(mins => first_minutes);
    NEW.resolution_due_at := NEW.created_at + make_interval(mins => resolution_minutes);
  ELSE
    NEW.first_response_due_at := NULL;
    NEW.resolution_due_at := NULL;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER conversations_set_sla
BEFORE INSERT OR UPDATE OF priority ON conversations
FOR EACH ROW EXECUTE FUNCTION set_conversation_sla();

CREATE TABLE sla_breaches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('first_response', 'resolution')),
  due_at timestamp with time zone NOT NULL,
  detected_at timestamp with time zone DEFAULT now() NOT NULL,
  notified_at timestamp with time zone,
  UNIQUE (conversation_id, kind)
);

CREATE INDEX sla_breaches_org_idx ON sla_breaches (org_id, detected_at DESC);
