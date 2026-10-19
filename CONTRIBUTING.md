# Contributing

Thanks for taking the time to improve ChasteDesk.

## Before you start

- Read `AGENTS.md` for repository conventions.
- Read `docs/ARCHITECTURE.md` for the shape of the system.
- For security issues, use a private GitHub security advisory instead of a public issue.

## Development workflow

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .
```

After editing a templ file:

```bash
go tool templ generate
```

If your change affects a page, include a screenshot or a short note about what changed visually.

## What makes a good change

- It solves one problem well.
- It validates input at the boundary.
- It scopes every database query by organization.
- It adds or updates a test that would fail without the change.
- It keeps the standard library first. New direct dependencies need a clear reason.
- It updates the relevant document when behavior changes.

## Commit style

Use a short imperative subject and explain the why in the body when it is not obvious.

```text
P12: add workspace invitations

Add one-time invitation links with role and expiry so a self-hosted
team can grow without an email service. Acceptance is transactional.
```

## Pull request checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `gofmt -l .` prints nothing
- [ ] `go tool templ generate` was run after template edits
- [ ] New API or job input is validated
- [ ] New database access is organization-scoped
- [ ] Security-relevant actions write audit rows
- [ ] User-facing copy is plain and specific
- [ ] No em dashes were added
- [ ] Documentation reflects the change

## License

By contributing, you agree that your contribution is licensed under AGPL-3.0-only.
