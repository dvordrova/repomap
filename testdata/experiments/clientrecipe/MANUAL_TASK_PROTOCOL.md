# Manual task protocol

Use the standalone preview to check whether the repository facts form a useful
change recipe. This protocol records expected observations; it is not a user
study and must not be reported as one.

## Setup

Serve only `testdata/experiments/clientrecipe` on a loopback address and open
`/preview/report.html`. Check once at a desktop viewport near 1440×900 and once
near 390×844.

## Walkthrough

1. On the landing page, identify the selected target and choose **Add an external client**.
   - Expected: six task choices are visible, but no recipe steps, source locators, or audit rows exist yet.
   - Expected: the coverage hint says 4 boundaries, 3 complete examples, and 6 excluded decoys.

2. On the recipe overview, identify the recommended example.
   - Expected: six steps are shown and Kubernetes is the only **Recommended to copy** example.
   - Expected: exactly three complete example cards are initially visible.

3. Open **Build the local boundary**, then choose **View evidence**.
   - Expected: the step explains construction plus the local wrapper before showing any path.
   - Expected: after one action, exact path, line, symbol, authority, and provenance rows appear.
   - Expected: **Open exact source** is the second action from the chosen step and has a locator-derived relative source href.

4. Return to the overview, choose **Show all 4**, and inspect Notifier.
   - Expected: Notifier is red/incomplete and is never marked recommended.
   - Expected: its missing roles are exactly Verification, Observability, and Failure policy.
   - Expected: the slot map has six groups; verification and observe/contain failure are the two visibly incomplete groups.

5. Return to the overview and open **Candidate audit**.
   - Expected: the audit is a separate drawer and was not present in the initial viewport or initial DOM.
   - Expected: it reports `10 observed = 4 admitted + 6 excluded` and lists exactly six closed exclusion reasons.
   - Expected: Escape closes the drawer and returns focus to its trigger.

6. Use the visible back links and browser back navigation between overview, step, example, and evidence states.
   - Expected: each hash state restores the coherent prior screen; ordinary browser back may also restore its previous scroll position.
   - Expected: keyboard focus is visible on controls, disclosures expose `aria-expanded`, and no information depends on hover.

## Visual checks

- No horizontal overflow, overlapping cards, clipped actions, or moving borders at either viewport.
- Evidence paths wrap without expanding the page width.
- The fourth example and the audit remain progressive disclosures.
- The page loads with zero external fonts, scripts, styles, images, or fetches.
