-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE scim_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  name text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  created_by text NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  last_used_at timestamp with time zone,
  revoked_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX scim_tokens_org_idx ON scim_tokens (org_id, created_at DESC);

CREATE TABLE scim_users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  org_id text NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
  user_id text NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  external_id text,
  user_name text NOT NULL,
  display_name text,
  active boolean DEFAULT true NOT NULL,
  role text DEFAULT 'agent' NOT NULL CHECK (role IN ('owner', 'admin', 'agent')),
  invitation_id text REFERENCES invitation(id) ON DELETE SET NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  UNIQUE (org_id, user_id),
  UNIQUE (org_id, user_name)
);

CREATE INDEX scim_users_org_idx ON scim_users (org_id, created_at DESC);
CREATE INDEX scim_users_external_idx ON scim_users (org_id, external_id);
