// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
)

type OnboardingState struct {
	ProviderConfigured bool
	KnowledgeAdded     bool
	TestSent           bool
	Complete           bool
}

type orgMetadata struct {
	Onboarding struct {
		TestSent bool `json:"testSent"`
		Complete bool `json:"complete"`
	} `json:"onboarding"`
}

func (r *Repo) OnboardingState(ctx RepoCtx) (OnboardingState, error) {
	state := OnboardingState{}
	var metadata *string
	if err := r.pool.QueryRow(ctx.Ctx, `select metadata from organization where id = $1`, ctx.OrgID).Scan(&metadata); err != nil {
		return state, err
	}
	if metadata != nil {
		var meta orgMetadata
		if err := json.Unmarshal([]byte(*metadata), &meta); err == nil {
			state.TestSent = meta.Onboarding.TestSent
			state.Complete = meta.Onboarding.Complete
		}
	}
	if err := r.pool.QueryRow(ctx.Ctx,
		`select exists(select 1 from provider_configs where org_id = $1)`, ctx.OrgID).Scan(&state.ProviderConfigured); err != nil {
		return state, err
	}
	if err := r.pool.QueryRow(ctx.Ctx,
		`select exists(select 1 from knowledge_sources where org_id = $1)`, ctx.OrgID).Scan(&state.KnowledgeAdded); err != nil {
		return state, err
	}
	return state, nil
}

func (r *Repo) SetOnboardingComplete(ctx RepoCtx, complete bool) error {
	var metadata *string
	if err := r.pool.QueryRow(ctx.Ctx, `select metadata from organization where id = $1`, ctx.OrgID).Scan(&metadata); err != nil {
		return err
	}
	var meta orgMetadata
	if metadata != nil {
		_ = json.Unmarshal([]byte(*metadata), &meta)
	}
	meta.Onboarding.Complete = complete
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx.Ctx, `update organization set metadata = $1 where id = $2`, string(encoded), ctx.OrgID); err != nil {
		return err
	}
	return r.WriteAudit(ctx, AuditInput{
		ActorType:  ActorUser,
		Type:       "org.admin",
		TargetType: "onboarding",
		Payload:    map[string]any{"complete": complete},
	})
}
