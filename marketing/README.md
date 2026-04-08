# Kaivue Marketing Site

Next.js 14 App Router scaffold for the Kaivue marketing site (`yourbrand.com`).
Tracked under [KAI-342](https://linear.app/kaivue/issue/KAI-342).

## Stack

- **Framework:** Next.js 14 (App Router) + TypeScript 5
- **i18n:** `next-intl` with EN / ES / FR / DE
- **CMS:** Sanity (`@sanity/client`) — placeholder project ID, swap via env
- **Analytics:** PostHog + Plausible (lazy-init, no-op until keys provided)
- **Forms:** HubSpot Forms API (stubbed at `app/api/lead/route.ts`)
- **Hosting:** Vercel (config template only, no real project linked)

## Local setup

```bash
cd marketing
cp .env.example .env.local
# fill in REPLACE_ME values when accounts are provisioned
npm install
npm run dev
```

Open http://localhost:3000 — you will be redirected to `/en`.

## Project structure

```
marketing/
  app/
    [locale]/
      layout.tsx          # NextIntlClientProvider, header/footer chrome
      page.tsx            # Homepage (hero + value props + CTAs)
      pricing/page.tsx
      careers/page.tsx
      legal/{terms,privacy,cookie}/page.tsx
      vs/[competitor]/page.tsx
    api/lead/route.ts     # HubSpot stub (logs only, returns 200)
    globals.css
  components/
    header.tsx            # Nav + language switcher
    footer.tsx
    cta-button.tsx        # Wires lead capture + analytics
  lib/
    sanity.ts             # @sanity/client wrapper
    posthog.ts            # Lazy PostHog init
    plausible.ts          # Lazy Plausible init
  messages/
    en.json               # EN strings (only locale with content)
    es.json               # placeholder
    fr.json               # placeholder
    de.json               # placeholder
  i18n.ts                 # next-intl config
  middleware.ts           # locale routing
```

## i18n contract (Seam #8)

**No hardcoded user-visible strings.** Every string flows through `next-intl`.
When adding a component:

1. Add the message key to `messages/en.json`
2. Reference it via `useTranslations('Namespace')`
3. Stub the same key (empty string is OK) in `es.json` / `fr.json` / `de.json`

The locale router under `app/[locale]` enforces this: pages without translations
will fall back to the EN bundle but never to inline strings.

## What is intentionally NOT done

This scaffold is part of a larger effort. The following items are deferred and
will be picked up in follow-up tickets:

- Real Sanity project + content models (KAI-343)
- Real HubSpot Forms API integration (KAI-345)
- PostHog / Plausible real keys (KAI-346)
- Vercel deploy + preview pipeline (KAI-347)
- Algolia DocSearch + Inkeep AI (KAI-349)
- Cloudinary asset pipeline (KAI-350)

Until then, all third-party keys are `REPLACE_ME` and clients no-op safely.

## Scripts

| Script           | Purpose                            |
| ---------------- | ---------------------------------- |
| `npm run dev`    | Next dev server on :3000           |
| `npm run build`  | Production build                   |
| `npm run start`  | Serve production build             |
| `npm run lint`   | ESLint via `eslint-config-next`    |
| `npm run typecheck` | `tsc --noEmit`                  |
