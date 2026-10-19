CREATE TABLE "jobs" (
	"id" text PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"data" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"status" text DEFAULT 'created' NOT NULL,
	"retry_limit" integer DEFAULT 0 NOT NULL,
	"retry_count" integer DEFAULT 0 NOT NULL,
	"retry_delay_seconds" integer DEFAULT 0 NOT NULL,
	"singleton_key" text,
	"run_at" timestamp with time zone DEFAULT now() NOT NULL,
	"started_at" timestamp with time zone,
	"completed_at" timestamp with time zone,
	"last_error" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX "jobs_singleton_unique" ON "jobs" ("name", "singleton_key") WHERE "singleton_key" IS NOT NULL AND "status" IN ('created', 'active', 'retry');
--> statement-breakpoint
CREATE INDEX "jobs_fetch_idx" ON "jobs" ("name", "run_at") WHERE "status" IN ('created', 'retry');
