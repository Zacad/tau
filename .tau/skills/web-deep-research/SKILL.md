---
name: web-deep-research
description: Perform comprehensive web research on a given topic and create a sourced report with concise prose. Uses subagent delegation, entity tracking, perspective diversification, reconciliation, and verification to maximize breadth, traceability, and coverage before synthesis.
---

# Web Deep Research

Comprehensive web research via orchestrated subagent delegation. Produces a broad, evidence-grounded report with concise prose where every factual claim includes a working source link.

The workflow must establish both **coverage of relevant entities** and **coverage of relevant perspectives** before synthesis. It should not narrow into evaluation or recommendations until exploration has challenged likely omissions, weak evidence, and missing viewpoints.

The goal is **wide research with proven information**, not a narrow recommendation memo. Recommendations may appear, but only as one section inside a complete research accounting.

## Core Artifacts

Save all artifacts under `docs/research/{topic-slug}/`:

- `00-candidate-ledger.md` — canonical tracked entity ledger and coverage tracker
- `01-exploration.md` — broad discovery artifact: perspectives used, entity-universe harvest, gaps, and saturation decision
- `02-{angle-name}.md` — one file per research angle
- `03-reconciliation.md` — pre-synthesis coverage, omission, and consistency audit
- `report.md` — comprehensive sourced report with concise prose
- `verification.log` — URL and semantic verification log

## Tracked Entity Rules

Use "candidate" broadly for the main entities being researched: products, vendors, policies, papers, tools, companies, options, or hypotheses depending on the topic.

- Track an entity when it may influence the final answer, comparison, or decision surface.
- Do not track incidental background nouns, source authors, or context-only organizations unless they become analytically relevant.
- Mark context-only comparators as `benchmark-only` instead of dropping them.
- Normalize aliases and variants, for example `VSETT 10+`, `Vsett 10 Plus`, and `VSETT 10+ Lite`.
- Low evidence is not a reason to omit an entity. Mark it `unresolved` or `low-confidence` and explain why.
- Repeated mentions across distinct perspective families increase the need to track or verify an entity.
- Some entities belong to broader clusters or groups. When a cluster becomes important, note whether it needs expansion before narrowing.
- Missing evidence is a reason to mark an entity `unresolved`, not to silently omit it.
- Every tracked entity must end with exactly one final disposition.

Allowed final dispositions:

- `recommended`
- `conditional`
- `excluded`
- `unresolved`
- `benchmark-only`

## Steps

### 1. Exploratory Search and Coverage Build (REQUIRED)

Before launching exploration, identify **at least 4 distinct perspective families** appropriate to the topic, such as:

- overview / landscape
- primary / official
- comparative / alternatives
- expert / practitioner discussion
- critical / skeptical
- evidence / benchmark / evaluation
- historical / timeline
- regional / local-context / language-specific

Then launch a `researcher` subagent to explore the topic broadly:

- `websearch` the topic across the chosen perspective families
- map the landscape and key concepts
- identify potentially relevant entities, clusters, and source ecosystems
- Save to `docs/research/{topic-slug}/01-exploration.md`

Before evaluation or recommendations, use exploration to build a raw universe of potentially relevant entities.

`01-exploration.md` must include:

- perspective families used
- source ecosystems covered
- raw entity universe
- aliases / variants / normalization notes
- why each entity may matter
- important clusters/groups needing expansion
- likely missing entities or perspectives to verify
- open questions and weak-evidence areas

Create `docs/research/{topic-slug}/00-candidate-ledger.md` immediately after exploration.

Ledger columns:

- entity
- aliases / variants
- entity type
- cluster / group if relevant
- discovered in
- perspective/source families
- evidence links
- evidence confidence: high / medium / low
- current status: candidate / recommended / conditional / excluded / unresolved / benchmark-only
- final disposition
- reason / open questions
- cluster expansion status if relevant

The ledger is the controlling artifact for synthesis. If an entity appears in exploration or angle research and may influence the answer, add it to the ledger or explicitly note why it is background-only.

Before moving on, explicitly assess whether exploration is broad enough.

`01-exploration.md` must answer:

- Which major perspective families were covered?
- Which important clusters/groups need expansion?
- Which likely important entities remain missing, weakly evidenced, or only singly sourced?
- Which benchmark, opposing, or context-critical entities may still be absent?
- Would a domain-savvy reader expect obvious entities, positions, or approaches not yet present?

Do not stop exploration simply because the report feels writable. Record whether recent exploration passes are still surfacing meaningful new entities or only duplicates. If major coverage gaps remain, continue exploration and update both `01-exploration.md` and the ledger.

### 2. Clarify Scope (REQUIRED)

Formulate 3-5 targeted questions to the user about scope, depth, constraints, exclusions, and what "comprehensive" should mean for this topic. Wait for response. If no response, proceed and document assumptions.

Use the clarification response to refine inclusion/exclusion criteria, relevant perspective families, and the expected breadth of the entity universe.

Update `00-candidate-ledger.md` if the clarification changes inclusion/exclusion criteria.

### 3. Design Research Angles (REQUIRED)

Define **minimum 3** distinct research angles. Each angle covers a unique perspective (comparative, technical, market, expert, practical, historical). No overlap.
Rewrite search queries from different perspectives.

Each angle must state:

- what it is responsible for proving or disproving
- which ledger fields it should update
- which perspective gaps it helps cover
- what would count as weak, conflicting, or missing evidence

Angles should not only deepen known material; they should also test whether important entities, positions, or clusters are missing.

### 4. Parallel Subagent Research (REQUIRED)

Launch a `researcher` subagent per angle. Each subagent:

- `websearch` then `webfetch` top results for detail
- Saves findings to `docs/research/{topic-slug}/02-{angle-name}.md`
- Every finding must include a source URL
- notes newly surfaced entities, clusters, or perspective gaps
- Ends the file with `Candidate/entity deltas`

Required `Candidate/entity deltas` sections:

- Added entities
- Updated entities
- Excluded entities
- Unresolved entities
- Benchmark-only entities

After subagents finish, merge every delta into `00-candidate-ledger.md`. Do not proceed to synthesis while angle findings contain entities or important clusters missing from the ledger.

### 5. Reconcile Coverage (REQUIRED)

Before writing the report, create `docs/research/{topic-slug}/03-reconciliation.md`.

The reconciliation must compare:

- `01-exploration.md`
- every `02-{angle-name}.md`
- `00-candidate-ledger.md`
- planned report sections

Required checks:

- every tracked entity has a ledger row
- aliases and variants are normalized
- every shortlist/promoted entity has a final disposition
- every final recommendation candidate exists in the ledger
- every low-confidence but relevant entity is retained as `unresolved` or `conditional`, not omitted
- benchmark-only entities are explicitly marked
- no entity discovered earlier can disappear without a recorded disposition
- major perspective families are adequately represented, or gaps are documented
- important clusters/groups discovered earlier have been expanded or explicitly limited
- no important repeated entity remains absent from the ledger or report
- benchmark, opposing, or context-critical entities are not silently omitted
- entities supported by only one perspective family are explicitly treated as low-confidence or unresolved
- a domain-savvy reader would not expect obvious missing entities, positions, methods, or options without explanation

If reconciliation finds gaps, update the ledger or perform more research before synthesis.

### 6. Synthesize Report (REQUIRED)

Read all research files. Write report to `docs/research/{topic-slug}/report.md`:

- comprehensive coverage first, recommendations second
- concise prose, but not narrow coverage
- 2-4 sentence executive summary
- scope and assumptions
- brief note on coverage method or perspective mix
- market/context map or landscape overview
- full candidate/entity accounting table from the ledger
- evidence confidence notes
- comparative analysis by research angle
- recommendations, if useful, as one section only
- considered but not selected
- unresolved / needs manual verification
- exclusions
- source-quality limitations
- explicit note on important unresolved coverage gaps, if any remain
- Every factual claim gets an inline source link
- Concise, direct language — no filler
- For commercial topics, link 2-3+ vendors per recommendation
- Flag weak or conflicting source material
- Do not collapse the report to only top recommendations
- Do not silently omit tracked entities, weak-source entities, rejected entities, or benchmark-only entities

### 7. Verify Sources and Coverage (REQUIRED)

`webfetch` a sample of report URLs to confirm accessibility. Replace broken links. Log results to `docs/research/{topic-slug}/verification.log`.

Also perform semantic verification:

- every final recommendation appears in `00-candidate-ledger.md`
- every tracked entity appears in `report.md` with a final disposition
- every shortlisted/promoted entity appears in recommendations, considered-but-not-selected, unresolved, benchmark-only, or exclusions
- every weak-source entity is retained with confidence notes, not dropped
- every important cluster/group is expanded or explicitly limited
- every important perspective gap is resolved or documented
- `03-reconciliation.md` has no unresolved coverage gaps

## Rules

- Every claim MUST have a source link — no exceptions
- Minimum 3 research angles
- Parallel subagents where possible
- Comprehensive coverage with concise prose
- Comprehensive coverage requires multiple distinct perspectives, not only paraphrases of one query style
- Build the entity universe before narrowing into evaluation or recommendations
- Document all assumptions
- The ledger is the source of truth for tracked entities
- No tracked entity may disappear silently
- If important clusters/groups appear, expand or explicitly limit them before synthesis
- Recommendations are optional and must not replace full research accounting

## Checklist

- [ ] 00-candidate-ledger.md created
- [ ] 01-exploration.md saved
- [ ] Multiple distinct perspective families used
- [ ] Entity universe harvested before evaluation
- [ ] Clarifying questions asked or assumptions documented
- [ ] ≥3 research angles defined
- [ ] All angle research files saved
- [ ] Candidate/entity deltas merged into ledger
- [ ] Important clusters/groups expanded or explicitly limited
- [ ] Coverage challenge completed
- [ ] Saturation decision recorded
- [ ] 03-reconciliation.md completed with no unresolved coverage gaps
- [ ] report.md written as a comprehensive sourced report with concise prose
- [ ] Every tracked entity has a final disposition in the report
- [ ] URL source verification completed
- [ ] Semantic coverage verification completed
