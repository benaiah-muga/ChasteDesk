-- SPDX-License-Identifier: AGPL-3.0-only
CREATE TABLE user_two_factor (
  user_id text PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
  secret_cipher text NOT NULL,
  secret_iv text NOT NULL,
  secret_tag text NOT NULL,
  key_version integer NOT NULL DEFAULT 1,
  enabled boolean DEFAULT false NOT NULL,
  recovery_codes jsonb DEFAULT '[]'::jsonb NOT NULL,
  confirmed_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE login_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  user_id text NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  expires_at timestamp with time zone NOT NULL,
  attempts integer DEFAULT 0 NOT NULL,
  used_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX login_challenges_expiry_idx ON login_challenges (expires_at);
CREATE INDEX login_challenges_user_idx ON login_challenges (user_id, created_at DESC);
