# Web Deep Research Skill Evaluation

## Scope
This report evaluates the **actual execution flow** of the `web-deep-research` skill in this session, using the research topic **"best off-road electric scooters under 10,000 PLN in May 2026"** as the test case.

The goal of this report is **not** to improve the market research itself, but to identify what the skill did well, where the flow broke down, and what should be improved in a future dedicated skill-design session.

---

## Executive Summary
The skill workflow was followed **structurally**, but not **reliably** end-to-end.

The process did complete all major stages:
1. exploratory search,
2. clarifying questions,
3. research angle design,
4. parallel subagent research,
5. synthesis,
6. source verification.

However, the final output exposed a serious weakness: **the synthesis step was not traceably grounded in the full candidate set discovered during earlier stages**. The clearest example is **KuKirin G4 Max**: it was discovered during exploration, promoted into the Polish shortlist, and still disappeared from the final recommendation report without being formally rejected.

This means the skill currently behaves more like **parallel note generation + freeform synthesis** than a robust research pipeline with enforced coverage, traceability, and explicit inclusion/exclusion decisions.

---

## What the Skill Did Correctly

### 1. The required high-level workflow was executed
The run did perform all required skill stages:
- `01-exploration.md` was created,
- clarifying questions were asked and answered,
- at least 3 research angles were defined,
- parallel researcher subagents were used,
- a final `report.md` was written,
- a `verification.log` was created.

This means the skill is **operationally usable** as a workflow scaffold.

### 2. Clarification improved the target significantly
The user constraints materially improved the research quality:
- new scooters only,
- mixed terrain with gravel/forest roads,
- only models available in Poland,
- priorities ordered as off-road performance -> 60 km minimum range -> service/reliability,
- public-road legality ignored.

This was a strong part of the flow. Without it, the research would likely have stayed too broad.

### 3. Research angles were meaningfully distinct
The three angles were useful and mostly non-overlapping:
- Polish market shortlist,
- technical suitability and range credibility,
- service / warranty / parts / reliability in Poland.

This is a good decomposition pattern for commercial product research.

### 4. The flow surfaced source-quality problems
The process did identify important source-quality issues:
- manufacturer and seller claims are marketing-heavy,
- independent real-range data is much stronger for some brands than others,
- service quality is easier to verify than real off-road performance,
- some models had incomplete or inconsistent technical data.

This is valuable because the skill did not blindly trust product pages.

---

## What Broke in the Workflow

## 1. No candidate ledger was maintained across stages
The biggest issue is the absence of a **single canonical candidate ledger**.

The workflow discovered candidates in multiple places, but there was no enforced structure saying:
- candidate name,
- discovered in which stage,
- current status,
- included / excluded / unresolved,
- why,
- evidence links.

Without that ledger, the final synthesis could silently drop items.

### Example: KuKirin G4 Max
KuKirin G4 Max is the clearest failure case.

It appeared in:
- `01-exploration.md` as part of the KuKirin portfolio,
- `02-rynek-polska-shortlista.md` as **shortlist item #1**.

The shortlist explicitly described it as a strong paper candidate with:
- Polish availability,
- dual motor,
- large battery,
- declared long range,
- strong price/performance positioning.

But in `report.md` it was **not carried through into the final recommendation set**, and it was **not explicitly rejected**.

This is not a research conclusion. It is a **pipeline failure**.

### Why this matters
A user reading only the final report would reasonably assume one of two things:
1. the model was never found, or
2. it was evaluated and rejected.

Neither is true.

The skill therefore failed to preserve **traceability from discovery -> shortlist -> final decision**.

---

## 2. Some candidates were discovered but never fully resolved
The same pattern affected other models too.

### Kamikaze K1 Max
Kamikaze K1 Max was discovered during exploration and appeared in the service/reliability analysis because 7way support, warranty, and parts availability were visible.

However, it was **not fully integrated into the main market/technical comparison** and did not receive a clear final status such as:
- recommended,
- conditionally recommended,
- excluded,
- insufficient evidence.

So the workflow partially processed the model, but did not close the loop.

### XRIDER F10 Pro
XRIDER F10 Pro appeared at exploration time through a Polish retail ranking context, but it was not subsequently turned into a full candidate evaluation or formal rejection.

That means the pipeline allowed a candidate to enter the system, then disappear without decision logging.

### Implication
The skill currently allows **candidate evaporation** between phases.

---

## 3. Synthesis was too freeform and not coverage-checked
The final `report.md` reads like a human synthesis, but it was not constrained by a completion check.

A robust synthesis step should answer:
- Did every shortlisted model appear in the final report?
- If not, where is the explicit exclusion note?
- Did every explored candidate receive one of a small set of statuses?
- Are unresolved candidates clearly marked as unresolved?

None of those checks were enforced.

As a result, the final report became a **narrative recommendation document**, not an auditable synthesis.

That is acceptable for casual research, but not for a skill whose explicit design goal is **deep, sourced, parallel, coherent, verified research**.

---

## 4. Parallel research angles were useful, but not reconciled strongly enough
The subagents produced useful angle-specific outputs, but the outputs were not reconciled with a mandatory merge discipline.

In practice, each angle created its own local truth:
- shortlist truth,
- technical truth,
- service truth.

The synthesis then cherry-picked across them without a formal cross-angle reconciliation step.

That is why a model can be strong in one angle and disappear later.

### Concrete consequence
- A model can be present in the market shortlist,
- present in service findings,
- missing from the final recommendation,
- and never formally excluded.

This is exactly what happened with **KuKirin G4 Max** and, in a different form, **Kamikaze K1 Max**.

---

## 5. The exploration stage was broader than the enforced decision surface
The exploration file successfully mapped the landscape, but the later stages did not guarantee that all meaningful exploration candidates were forced into the decision funnel.

In this run, exploration included:
- KuKirin G4 / G4 Max,
- Kamikaze K1 Max,
- XRIDER F10 Pro,
- broader brands and benchmark models.

But the downstream workflow did not require:
- every exploration candidate to be explicitly advanced, parked, or rejected.

So exploration worked as a map, but not as a controlled intake mechanism.

---

## 6. The verification step checked URL accessibility, not analytical completeness
The `verification.log` confirmed that sampled URLs were reachable and that key cited claims were visible.

That is useful, but it only validates **link availability**, not:
- whether all important models were covered,
- whether all final recommendations are consistent with prior files,
- whether omissions are justified,
- whether contradictory evidence was resolved.

So source verification worked technically, but not semantically.

---

## Root Causes

## 1. The skill has no mandatory entity-tracking mechanism
The workflow relies on documents, not on a candidate registry.

That means the process is document-centric rather than decision-centric.

## 2. The synthesis prompt optimizes for terseness and coherence, not completeness
The skill strongly encourages a concise final report, but does not strongly require:
- full candidate accounting,
- explicit disposition for every serious candidate,
- auditability back to prior notes.

## 3. Parallel subagents generate angle-local outputs without a forced merge contract
Each subagent answered its own question well enough, but no step required the merger to reconcile all entity mentions into one decision table.

## 4. Weak-source candidates are easy to drop silently
When evidence quality is lower, the current flow tends to omit candidates instead of marking them:
- "insufficient evidence",
- "seller-only evidence",
- "shortlisted but unresolved",
- "not enough independent data".

That is a major design weakness.

---

## Specific Discoveries from This Session

## Discovery 1: KuKirin G4 Max exposes a traceability gap
This is the most important finding from the session.

**Observed path:**
- discovered in exploration,
- promoted into the Polish shortlist,
- described as a strong candidate on paper,
- omitted from final report,
- not explicitly rejected anywhere in the final synthesis.

**Interpretation:**
The skill can lose a strong candidate during synthesis even after it has already passed an intermediate filter.

**Why this is the best test case for improving the skill:**
Because this is not a subtle judgment difference. It is a visible procedural inconsistency.

---

## Discovery 2: Kamikaze K1 Max exposes partial-angle processing
Kamikaze K1 Max was processed from the **service/warranty/parts** angle, but not carried into the final comparative recommendation logic.

**Interpretation:**
The skill can process a model deeply in one angle while failing to elevate it into the global candidate evaluation.

This suggests the merge step is under-specified.

---

## Discovery 3: XRIDER F10 Pro exposes intake without closure
XRIDER F10 Pro entered via exploration but did not receive full follow-through.

**Interpretation:**
The exploration stage can introduce entities that later vanish because there is no enforced closure rule.

---

## Discovery 4: Strong benchmark models dominated synthesis even when they did not cleanly fit the user brief
The final report leaned heavily on **Nami Klima** and **VSETT 10+** as technical benchmarks because they had stronger independent testing.

That is analytically reasonable, but it also created a bias: models with better documentation received more narrative weight than models that may have fit the actual buying brief more closely.

This is a subtle but important skill behavior:
- **high-evidence models dominate the narrative**,
- **lower-evidence but still relevant market candidates risk underrepresentation or omission**.

That is not always wrong, but it must be made explicit.

---

## Recommendations for Improving the Skill

## 1. Add a mandatory candidate ledger file
The skill should create a file such as:
- `00-candidate-ledger.md`

Each candidate should include:
- name,
- source of discovery,
- price status,
- availability status,
- evidence strength,
- current status: included / excluded / unresolved,
- reason,
- links.

This alone would likely have prevented the KuKirin G4 Max failure.

## 2. Require explicit disposition for every serious candidate
The skill should enforce this rule:

> Every candidate that appears in exploration or angle files must end with one explicit disposition in the final synthesis.

Allowed statuses could be:
- recommended,
- conditionally recommended,
- excluded,
- unresolved due to insufficient evidence.

## 3. Add a reconciliation step before final synthesis
Before writing `report.md`, the workflow should run a dedicated merge pass that asks:
- Which models appeared anywhere?
- Which models are missing from the final draft?
- Why are they missing?
- Is that omission justified and logged?

## 4. Add a coverage checklist for synthesis
The final synthesis prompt should explicitly require:
- no shortlisted model may disappear silently,
- every omitted model must be named and rejected explicitly,
- every recommendation must reference both strengths and evidence confidence.

## 5. Distinguish "low evidence" from "low quality"
The current flow risks collapsing these into omission.

The skill should instead explicitly separate:
- technically weak models,
- overpriced models,
- poor-service models,
- relevant but under-documented models.

This matters because **under-documented does not mean irrelevant**.

## 6. Extend verification beyond URL reachability
Verification should also include a **consistency audit**:
- sample-check URLs,
- check that every final recommendation exists in source files,
- check that every shortlisted candidate has a final status,
- check for any candidate mentioned earlier but absent later.

## 7. Consider a final reviewer subagent
A reviewer subagent could be asked:
- Find any candidate discovered earlier that vanished from the final report.
- Find any recommendation not properly justified by source coverage.
- Find any unresolved contradictions.

That would be a strong fit for this skill.

---

## Proposed Skill-Level Acceptance Tests
If this skill is improved later, it should probably be tested against cases like this one.

### Test A: Candidate persistence
If a model appears in exploration and shortlist, it must either:
- appear in final recommendations, or
- appear in final exclusions.

### Test B: Partial-angle entity detection
If a model appears in only one angle file, the final synthesis must still classify it explicitly.

### Test C: Weak-source handling
If a model is relevant but weakly documented, the final report must mark it as:
- unresolved,
- seller-data only,
- needs manual verification,
not silently omit it.

### Test D: Consistency audit
A reviewer should be able to compare:
- exploration,
- angle files,
- final report,
and find no silent candidate loss.

---

## Final Assessment
The skill is **good as a workflow skeleton**, but **not yet reliable as a traceable deep-research system**.

Its current strengths are:
- fast decomposition,
- good use of clarifying questions,
- useful parallel angle research,
- decent source sampling,
- strong output structure.

Its current weaknesses are:
- no canonical candidate tracking,
- no mandatory inclusion/exclusion ledger,
- weak reconciliation across subagent outputs,
- narrative synthesis that can silently drop important candidates,
- verification that checks links but not reasoning completeness.

### Bottom line
The session successfully tested the skill and revealed a concrete, reproducible design flaw:

> A candidate can be discovered, shortlisted, and still disappear from the final report without explicit rejection.

**KuKirin G4 Max** is the clearest example and should be treated as the primary reference case when improving the skill in a future session.
