-- SPDX-License-Identifier: AGPL-3.0-only
CREATE UNIQUE INDEX IF NOT EXISTS user_email_lower_uq ON "user" (lower(email));
CREATE UNIQUE INDEX IF NOT EXISTS member_org_user_uq ON member (organization_id, user_id);
