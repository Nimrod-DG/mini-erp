# Decision 003 — UTC everywhere, tenant timezone for business dates

> Binding. Read in Phase 0 and again in Phase 5 before document numbering.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 2.5 Time and timezone

**Every environment runs in UTC. This is not a preference — a mismatch between a local container in Asia/Jakarta and a deployed container in UTC produces wrong data, not just odd-looking timestamps.**

#### 2.5.1 Why it matters here specifically

Storage is the easy part: `TIMESTAMPTZ` records an absolute instant, so `now()` is correct regardless of the server's timezone. The damage happens wherever an instant is converted to a **date** or a **month**, because that conversion uses the session timezone.

The concrete trap in this system is **document numbering** (Section 8.1). The sequence period is `YYYYMM`. A requisition created at **23:30 on 31 July in Jakarta** is **16:30 on 31 July UTC** — same month, fine. But one created at **00:30 on 1 August in Jakarta** is **17:30 on 31 July UTC**: period `202508` on your laptop, period `202507` in production. Two environments would allocate from different counters for the same instant, and month-end reports would disagree about which month a document belongs to.

The same conversion appears in `expected_at DATE`, low-stock and dashboard date filters, and any future period close.

#### 2.5.2 Pin everything to UTC

| Layer | Setting |
|---|---|
| Postgres container | `TZ=UTC` and `PGTZ=UTC` in the environment |
| Database roles | `ALTER ROLE erp_app SET timezone = 'UTC';` (and `erp_admin`, `erp_migrate`) |
| Go container | `TZ=UTC`; `distroless` images have no tzdata, so this is already effectively true |
| Go code | Use `time.Now().UTC()`. Never `time.Local` |
| CI | `TZ=UTC` in the workflow environment |

Setting it on the **role** rather than only the container is what makes it robust: the setting travels with the connection, so it holds on a managed host whose containers you do not control.

Test group J asserts this, and J1 must be run against the **deployed** database too — not just locally. That is the check that would have caught the mismatch.

#### 2.5.3 Business dates belong to the tenant, not the server

Pinning infrastructure to UTC fixes consistency but raises a business question: an Indonesian company posting a receipt at 08:00 on 1 August WIB should see it dated **1 August**, not 31 July.

So tenants carry their own timezone — `tenants.timezone`, defined in Section 6.1.

Two operations — and only two — use it:

1. **Document period allocation** (Section 8.1): `to_char(now() AT TIME ZONE t.timezone, 'YYYYMM')`
2. **Date display and date-range filters** in the UI

Everything else stays in UTC. Timestamps are stored, compared, sorted, and logged in UTC; the tenant timezone is applied only at the point where an instant becomes a business date.

The frontend renders timestamps in the **tenant's** timezone, not the browser's. A manager travelling abroad should not see their company's documents shift by a day.
