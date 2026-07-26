# Start here

This folder replaces `ERP_BUILD_PLAN.docx`. Nothing else is needed.

## What you do (once, ~2 minutes)

1. Create the repo: `mkdir mini-erp && cd mini-erp && git init`
2. Copy `CLAUDE.md` and the `docs/` folder into it, at the root:

   ```
   mini-erp/
   ├── CLAUDE.md          ← Claude Code reads this automatically, every session
   └── docs/              ← everything else
   ```

3. Open Claude Code in that directory and say:

   > Read CLAUDE.md and start Phase 0.

That's it. You do not need to tell it which files to read — `CLAUDE.md` sends it
to `docs/PROGRESS.md`, which sends it to the current phase brief, which names the
three or four reference files that phase needs.

Every session after the first one is the same sentence: **"Read CLAUDE.md and
continue."**

## The only file *you* should read

[`docs/AUDIT.md`](docs/AUDIT.md) — the sixteen problems found in the original
plan and how each was fixed. Four of them would have stopped the build outright.
Worth twenty minutes, because you may disagree with a fix and it is your
architecture. Everything else in `docs/` is written for the agent.

## How this is different from the single document

The original told Claude Code to *"read this whole document before writing any
code"* — about 45,000 tokens of specification, of which maybe 8% is relevant to
any given phase. That is expensive, and it degrades quality: a model holding the
colour palette and the RLS policy template in the same context starts blending
them.

Here, the working set is:

| Always loaded | ~100 lines — invariants, naming contracts, scope discipline |
| Per session | 1 phase brief + the 3–5 reference files it names |
| Never loaded unless needed | deployment (Phase 9), audit log (Phase 11), the narrative |

`docs/PROGRESS.md` is what makes it work across sessions: each session ends by
recording what now works, which tests are green, and the single next action. A
fresh session reads that instead of re-reading the specification.

## Layout

```
CLAUDE.md                       auto-loaded core
docs/
├── README.md                   the doc map
├── PROGRESS.md                 running state — read first, append last
├── AUDIT.md                    what was wrong in v2.0 and what changed
├── 00-scope.md                 MVP boundary
├── acceptance-test.md          the 25-step gate
├── narrative.md                portfolio write-up material
├── phases/                     12 build briefs, one per phase
├── reference/                  18 lookup files
├── decisions/                  3 rationale docs
└── post-mvp/                   audit log schema, Phase 11 only
```

## One habit worth keeping

**End each phase in its own session.** The context that built Phase 4 is not the
context Phase 5 needs, and carrying it over is how naming drift starts. Phase 5 is
large enough that its brief splits it across two sessions on purpose.
