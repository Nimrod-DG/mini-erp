# Phase 10 — Documentation · *post-MVP*

**MVP:** no · **Estimate:** 2h · **Depends on:** Phase 9

> **Priority changed 2026-07-27.** The deliverable is now the **portfolio case
> study**, not a README. The plan lives in the other repository:
> **`D:\Work\lw-sports-portfolio\PLAN-mini-erp.md`** — read that, and start a
> session in *that* directory rather than this one. It is self-contained.
>
> The repository README stays worth writing, but it is downstream of the case
> study and reuses the same material.

## Load only these

1. **`D:\Work\lw-sports-portfolio\PLAN-mini-erp.md`** — the executable plan.
2. [`../narrative.md`](../narrative.md) — in full.
3. [`../decisions/`](../decisions/) — all three. They are the source material for
   the write-up.

## Build

A portfolio case study with screenshots of the running application, then a
README with an architecture diagram, the RLS explanation and the permission
model.

**Screenshots come from the app running locally**, against a rebuilt seeded
database — the deployment is not finished (Phase 9: the database and frontend
are live, the API is not, and the blocker is a payment method rather than
anything in the code). Say **deploy-ready, not deployed**, and link the GitHub
repository rather than the half-deployed frontend, which stops at the login
screen because it was built against a placeholder API URL.

Two things to get right, because they cost credibility rather than marks:

- **Never describe Go as encrypted.** Binaries are compiled, not encrypted, and Go
  is comparatively easy to reverse-engineer. The defensible reasons are memory
  safety, a small container attack surface, and static typing. Encryption here is
  TLS in transit and provider-managed encryption at rest.
- **Say "modular monolith", not "microservices".** Splitting the frontend from the
  backend is deployment topology, not service decomposition. The monolith is the
  point: the atomic cross-module write is exactly what microservices would cost.

Be straightforward about the limits too — Finance is a stub, no invoicing or
payment cycle, single-level approvals, no period close. Scoping honestly reads
better than overclaiming and leaves room to talk about what comes next.

## Done when

- [ ] A reader who has never seen the code can explain the isolation model from
      the case study alone
- [ ] Screenshots include the cross-module confirmation panel
- [ ] The screenshot set covers all three modules plus tenants and users —
      procurement, inventory, finance, `/admin/tenants`, `/settings/users` — not
      just the dashboard
- [ ] The portfolio's existing LW Sports case study still works unchanged
