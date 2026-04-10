# Kaivue Recording Server Docs Portal

This directory contains the Mintlify scaffold for the Kaivue Recording Server documentation portal (`docs.yourbrand.com`).

## Status

Local scaffold only. Authoring lives here as plain `.mdx` and `mint.json`. **Mintlify signup, hosted preview, and deployment are deferred to end-of-roadmap** (see `docs/superpowers/specs/2026-04-07-v1-roadmap.md`). Do **not** sign up for Mintlify, run `mintlify deploy`, or install the Mintlify CLI as part of routine work on this scaffold.

## Local preview (when permitted)

When the team is ready to preview the site locally, install the Mintlify CLI on a developer workstation and run:

```bash
# DO NOT run this as part of CI or as part of KAI-349 itself.
npm install -g mintlify
cd docs-portal
mintlify dev
```

This serves the docs at `http://localhost:3000`. No account is required for local preview.

## Layout

```
docs-portal/
  mint.json                  # Mintlify configuration (placeholders for keys)
  api-reference/
    openapi.yaml             # Placeholder OpenAPI 3.1 shell
    introduction.mdx
  end-user/                  # End-user-facing guides
  customer-admin/            # Customer administrator docs
  integrator/                # Integrator / partner docs
  developer/                 # Developer / API docs
  operator/                  # SOC operator docs
  hardware/                  # Hardware and installer docs
  compliance/                # Compliance & audit docs
```

The 7 audience sections come from spec § 19.2 of the V1 roadmap (`docs/superpowers/specs/2026-04-07-v1-roadmap.md`).

## Placeholders

All API keys, color tokens, and external URLs in `mint.json` use the literal `REPLACE_ME` or `placeholder_*` markers. **Do not commit real API keys** to this directory. Real keys will be wired in via environment substitution at deploy time once the Mintlify account is provisioned.

## Multi-language

Languages declared: EN (default), ES, FR, DE. Translation pipeline is TBD.

## Search & AI

- **Algolia DocSearch**: placeholder credentials in `mint.json`. Apply for DocSearch (or self-host) once the docs corpus is indexable.
- **Inkeep AI**: placeholder credentials in `mint.json`. Provision the Inkeep workspace once content stabilizes.

## Owner

Web frontend team. See `.claude/agents/web-frontend.md`.
