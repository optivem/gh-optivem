# 2026-06-17 10:01:00 UTC — Rehearsal corpus: real test-instance external system + refine-ticket path

## TL;DR

**Why:** Two BPMN paths are entirely unexercised by any rehearsal. (1) The `real-kind == test-instance` branch of the contract-test flow never runs because every shop external system (`erp`, `clock`) is pinned `real-kind: simulator` — a documented coverage gap. (2) The `refine-ticket` TOP path (backlog grooming) has no rehearsal at all, and in fact has **no CLI entry point** — `gh optivem` only exposes `implement`.
**End result:** A new JSONPlaceholder-backed external system declared `real-kind: test-instance` plus a story ticket that touches it, closing the contract-test gap; and a scoped decision (with a built entry point, if approved) for rehearsing the refine path.

## Outcomes

What we get out of this — the goals and deliverables:

- A real, always-available external system (JSONPlaceholder, `https://jsonplaceholder.typicode.com`) declared `real-kind: test-instance` in the shop config(s), so contract tests hit a live shared instance instead of an authored simulator.
- A story ticket that consumes that external system, driving the `test-instance` contract branch (collapse to a single contract-real pass-verify, **no** real-simulator authoring — the branch the simulator path never reaches).
- The contract-test coverage gap noted in `CONTRIBUTING.md` closed, and the note updated to reflect that the branch is now exercised.
- A clear decision on the `refine-ticket` rehearsal: either build a `gh optivem refine` (or equivalent) entry point + harness support and add an un-refined ticket, or explicitly defer it — captured here rather than silently dropped.

## ▶ Next executable step (resume here)

This resumes under `/refine-plan` — both items need a design decision before any mechanical edit. For item 5: confirm the **identity and domain framing** of the JSONPlaceholder-backed external system (recommendation: a read-only "customer directory" / CRM reading `/users`) and which config variants get it. For item 6: decide whether to build a refine CLI entry point now or defer (recommendation: defer — it is a separate feature, not a corpus addition). Once item 5's framing is settled, the first executable unit is **Step 5a** (register the external system in the shop config).

## Steps

### Item 5 — JSONPlaceholder external system as a real `test-instance`

- [ ] 5a. Register a new external system in the shop config(s) (`systems.yaml` / the `gh-optivem-*.yaml` external-systems registry), `real-kind: test-instance`, pointing at `https://jsonplaceholder.typicode.com`. **Recommended framing:** a read-only "customer directory" external system backed by `/users` (stable, deterministic, public — ideal for a shared test instance).
- [ ] 5b. Confirm the `test-instance` real-kind is supported end-to-end in the runtime (the contract-test flow's `real-kind == test-instance` branch — single contract-real pass-verify, no real-sim authoring). If any runtime/config wiring is missing, capture it as a sub-step before authoring the ticket.
- [ ] 5c. Author a **story** ticket in `optivem/shop` (Gherkin AC, not Checklist) that reads data from the new external system — e.g. surface a customer's name on an order/confirmation by fetching it from the customer directory. The story must trip `external-driver-port-changed` so the contract-test excursion runs.
- [ ] 5d. Add the ticket to `DEFAULT_TICKETS` in `scripts/atdd-rehearsal-loop.sh` (new `--- external system: real test-instance ---` group) and a matching `CONTRIBUTING.md` subsection that calls out this is the `test-instance` branch (contrast it with #72's simulator branch).
- [ ] 5e. Rehearse under `gh-optivem-monolith-java.yaml`. Confirm from the trace it took the `test-instance` branch: a single contract-real verify that PASSES against the live JSONPlaceholder instance, with **no** real-simulator authoring step (vs #72's verify-fail real → author real-sim → verify-pass real → stub red→green).
- [ ] 5f. Update the "Known coverage gap" note in `CONTRIBUTING.md` (currently saying the `test-instance` branch is never exercised) to record that this ticket now covers it.

### Item 6 — refine-ticket rehearsal (blocked: no CLI entry point)

- [ ] 6a. **Decision gate.** `refine-ticket` / `refine-backlog-item` exist in the BPMN but have no CLI surface (`gh optivem` exposes only `implement`; `implement --target` is a closed enum `test | driver-adapter | system` and cannot name an arbitrary process). Decide: build an entry point now, or defer. **Recommended: defer** — this is a product feature (expose refinement), not a corpus addition, and belongs in its own implementation plan.
- [ ] 6b. *(only if building)* Add a CLI entry point that runs the `refine-ticket` process (e.g. `gh optivem refine <issue>`), invoked against a READY-precondition-free (un-refined) ticket.
- [ ] 6c. *(only if building)* Extend `scripts/atdd-rehearsal.sh` with a `--refine` mode (or a sibling `atdd-refine-rehearsal.sh`) that runs the refine command instead of `implement --issue`, since the current script hardcodes `implement`.
- [ ] 6d. *(only if building)* Author an un-refined shop ticket (raw idea, no AC/Checklist yet) for the refine walk to groom, add it to the corpus, and rehearse — confirming it walks `refine-ticket` → `refine-backlog-item` → `refine-acceptance-criteria` and ends with the ticket READY.

## Open questions

- **External-system identity & domain fit** — JSONPlaceholder serves generic resources (`/users`, `/posts`, `/todos`), none shop-native. Recommended framing is a read-only customer-directory/CRM over `/users`; confirm this is plausible enough for the shop domain, or pick another resource/framing.
- **Config spread** — does the new external system go into all stack configs (`monolith`/`multitier` × `java`/`dotnet`/`typescript`) or only the one(s) used for rehearsal? Recommend: add to all to keep configs consistent, but rehearse only under `gh-optivem-monolith-java.yaml`.
- **`test-instance` runtime support** (5b) — whether the `real-kind: test-instance` branch is fully wired in the runtime today, or needs implementation, is unverified. Resolve before authoring 5c.
- **Refine path scope** (Item 6) — confirm defer vs build. If build, it should likely graduate into its own implementation plan rather than living under the rehearsal-corpus umbrella.
