# Phase 10 — Documentation · *post-MVP*

**MVP:** no · **Estimate:** 2h · **Depends on:** Phase 9

## Load only these

1. [`../narrative.md`](../narrative.md) — in full.
2. [`../decisions/`](../decisions/) — all three. They are the source material for the write-up.

## Build

A README with an architecture diagram, the RLS explanation, the permission model,
and screenshots. Then the portfolio write-up.

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
      the README alone
- [ ] Screenshots include the cross-module confirmation panel
