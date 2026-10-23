# 08-auth — Auth

- **W3 detail mode:**
  - One authentik OIDC app `mountain` through `/authentik-oidc`.
  - Groups claim: `platform-admins` → detail frames, plant and the eraser (`mark.delete`); `mountain-visitors` → plant; anonymous → view.
- **W4 hardening** (one PR each, under ADR-B, before visitor planting opens to the public):
  1. `mountain-enrollment` flow cloned from `authentik-blueprint-wird-enrollment.yaml`, keeping Wird's **email-verification stage** (shared sender, R3). Add a **Turnstile captcha stage** (supported in authentik 2026.8.0). The widget is created through the Cloudflare API, or documented as the one manual step in ADR-B. Keys go to Infisical and are injected into the blueprint via `!Env`. New users land only in `mountain-visitors`.
  2. **Binding gate:** a script that checks every authentik Application has a policy binding. It runs against committed blueprints **and the live authentik API** (covers restores and provisioner-created apps). `AppAccessWithoutBindings` defaults to true; DECISION.md:192-196.
  3. **Grafana:** set `role_attribute_path` to `contains(groups[*], 'platform-admins') && 'Admin' || ''` **plus** `role_attribute_strict: true` (`values/grafana/values.yaml:171`; strict alone is a no-op).
  4. ~~"Sign in to play" links to `/if/flow/mountain-enrollment/`~~ R8 deploy C: gone. The site is behind the
     `mountain-gate` forwardAuth and enrollment is invitation-only (infra-bootstrap ADR-0065); the page signs in to the
     relay by itself (`toys.js autoSignIn`, once per 10 min) and keeps a plain "Sign in" link for a later try.
  5. **Moderation:** handle denylist, 3–20 chars, `[a-z0-9_-]`. Admin-only `mark.delete` in detail mode. The `VISITORS_ENABLED` kill switch. SMTP2GO quota watch (a weekly check, or an alert if an API is available).

