# 03 — Reconciliation and coverage audit

## Inputs compared

- `01-exploration.md` — broad discovery and initial entity universe.
- `02-price-spec-eu.md` — EU/PL price, availability, hard constraints.
- `02-terrain-performance.md` — terrain performance, ride quality, braking, suspension and real range.
- `02-reliability-service.md` — reliability, warranty, parts, water and ownership risk.
- `02-omissions-alternatives.md` — omission challenge, alternatives and benchmarks.
- `00-candidate-ledger.md` — controlling artifact updated after angle deltas.
- Planned report sections: executive summary, scope, method, landscape, full ledger accounting, evidence notes, comparative analysis, recommendations, considered/not selected, unresolved, exclusions, limitations and verification.

## Clarification impact

User clarified that the intended use is mixed roads, gravel and forest paths; off-road tires should be present or at least explicitly handled; legal status is not important; evaluation should be technical and purchasing-focused; the 10,000 zł budget is vehicle-only; purchases from the EU are accepted but non-EU purchases are excluded; large range, terrain power and minimum 120 kg load are required.

The ledger was updated to reflect these inclusion rules. Models that are legal/city oriented but weak in terrain power were downgraded or marked benchmark-only/excluded. UK-only or US-oriented sources no longer count as sufficient purchase evidence.

## Required checks

### Every tracked entity has a ledger row

Pass. Every entity from exploration and all angle deltas that may influence the answer has a row in `00-candidate-ledger.md`: Teverun Fighter Mini Pro, Kaabo Mantis King GT, VSETT 10+, NAMI Klima, KuKirin G4 Max, KuKirin G3 Pro, Hiley Tiger 10 V5, Kaabo Wolf Warrior X GT, Teverun Blade GT II, Segway GT2P, Inmotion RS Lite, Apollo Phantom, Dualtron Mini, Ausom, Nanrobot, YUME, Ruptor R1, Kamikaze, XRIDER, OKAI Panther, Segway Max G2, plus added Ruptor R9 Rage, Teverun Blade Mini Ultra, Dualtron Victor, Mukuta 10 Plus, ECO Speed 10X V2, Zero 10X, LAOTIE Ti30, Joyor S10-S, Techlife, Navee/city class and Motus Daytona.

### Aliases and variants are normalized

Pass with documented variant caveats. Teverun Blade GT II vs GT II+ and Hiley Tiger 10 V5 52V vs V5 Performance 60V remain variant-sensitive, so the ledger marks them conditional rather than silently merging specs. VSETT Lite/Super/Pro and NAMI Klima/Klima Max are normalized as model families with budget caveats.

### Every shortlist/promoted entity has a final disposition

Pass. Recommended: Kaabo Wolf Warrior X GT, KuKirin G4 Max, Hiley Tiger 10 V5. Conditional: Teverun Blade GT II, Teverun Fighter Mini Pro, NAMI Klima, Kaabo Mantis King GT, VSETT 10+, Teverun Blade Mini Ultra, KuKirin G3 Pro. Benchmark-only/excluded/unresolved entities are explicitly marked.

### Every final recommendation candidate exists in the ledger

Pass. Kaabo Wolf Warrior X GT, KuKirin G4 Max and Hiley Tiger 10 V5 all appear in the ledger with `recommended` final disposition.

### Every low-confidence but relevant entity is retained as unresolved or conditional, not omitted

Pass. Ausom Gallop, OKAI Panther, LAOTIE Ti30 and Ruptor R9 tire-fit questions are retained as unresolved. ECO Speed and Mukuta are benchmark-only rather than omitted.

### Benchmark-only entities are explicitly marked

Pass. Segway GT2P, Inmotion RS Lite, Dualtron Victor, Mukuta 10 Plus, ECO Speed 10X V2, Segway Max G2, Zero 10X and Navee/city class are marked benchmark-only where appropriate.

### No entity discovered earlier disappeared without a recorded disposition

Pass. Entities from the raw universe and angle deltas are present with final dispositions in the updated ledger.

### Major perspective families are represented or gaps documented

Pass. Sources cover official specs, PL/EU shops and prices, independent reviews/tests, forum/ownership risk, parts/service/warranty and omission challenge. Gaps remain for private Polish Facebook ownership groups and direct seller confirmation of some payload/tire details, and these will be documented as limitations.

### Important clusters/groups expanded or explicitly limited

Pass. Premium 10" dual, 11–12" off-road, budget/value, PL semi-terrain, markowe street/all-road, hyper benchmarks and city/comfort anti-benchmarks were expanded or limited. Ruptor was expanded from R1 to R9 Rage. Teverun was expanded to Blade Mini Ultra. VSETT-like benchmarks were expanded to Mukuta/ECO Speed/Zero 10X.

### No important repeated entity remains absent from the ledger or report plan

Pass. Repeated entities Teverun, Kaabo, VSETT, NAMI, KuKirin and Hiley are included in the ledger and will be discussed in the report.

### Benchmark, opposing or context-critical entities are not silently omitted

Pass. Segway GT2P/Max G2, Inmotion RS Lite, Dualtron Victor/Mini, Apollo Phantom, city comfort models and marketplace beasts are retained as benchmarks/exclusions.

### Entities supported by only one perspective family are treated as low-confidence or unresolved

Pass. LAOTIE, ECO Speed, OKAI Panther, Ausom, Joyor, Motus, Navee/city class and some PL semi-terrain entities are marked low/medium-low, unresolved, excluded or benchmark-only.

### Domain-savvy reader obvious omissions

Partially pass. Omission search added Ruptor R9 Rage, Teverun Blade Mini Ultra, Dualtron Victor, Mukuta 10 Plus, ECO Speed 10X V2 and Zero 10X. Remaining potential omissions are niche marketplace beasts and used premium models; used models are outside scope because the topic and price validation are for new vehicle purchase from PL/EU.

## Saturation decision

The research is saturated enough for a concise comprehensive report. Additional searches still surface long-tail or marketplace models, but they do not materially challenge the core finalist set because they lack stable EU support, verified off-road tires, payload proof or current availability under 10,000 zł.

## Planned report coverage

The report will include:

1. Executive summary.
2. Scope and assumptions based on user clarification.
3. Perspective mix and source-quality note.
4. Landscape map by clusters.
5. Full candidate/entity accounting table derived from `00-candidate-ledger.md`.
6. Comparative analysis by price/spec, terrain performance and ownership risk.
7. Recommendations as one section only.
8. Considered but not selected.
9. Unresolved/manual verification list.
10. Exclusions and benchmark-only models.
11. Source limitations and coverage gaps.

## Remaining unresolved coverage gaps

No unresolved coverage gap blocks synthesis. Remaining unresolved items are model-specific due-diligence points, not universe-coverage gaps:

- confirm exact SKU and payload for Teverun Blade GT II from a chosen PL seller;
- confirm nośność and factory/off-road tire setup for Teverun Fighter Mini Pro, Kaabo Mantis King GT and Teverun Blade Mini Ultra;
- confirm whether NAMI Klima offer under 10,000 zł is still available and whether the selected source states 120 kg payload;
- confirm Ruptor R9 Rage tire tread/off-road tire compatibility before treating it as a finalist;
- inspect any selected model physically or through seller confirmation for waterproofing and warranty exclusions before purchase.

## Reconciliation conclusion

Proceed to synthesis. The ledger has final dispositions for every tracked entity, important additions from angle research have been merged, and no major perspective family is missing. Remaining gaps will be documented in the report as purchase-time verification items rather than solved by more broad exploration.
