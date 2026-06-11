#!/usr/bin/env bash
set -euo pipefail

# Wrap body in `{ ... } && exit` so bash parses the whole script up-front and
# never re-reads from disk mid-run (same reason as atdd-rehearsal.sh — a long
# implement walk can outlive an edit to this file).
{

# Runs atdd-rehearsal.sh over a list of tickets unattended, one after another.
# Each ticket gets its own fresh worktree (built + cleaned + implemented +
# torn down) before the next starts — there is no parallelism, by design: the
# rehearsal rebuilds the binary and cleans local docker state each time, so
# overlapping runs would race on both.
#
# Usage:
#   bash atdd-rehearsal-loop.sh [--config <yaml>] [ticket ...]
#
#   ticket:    one or more issue numbers / URLs forwarded to atdd-rehearsal.sh.
#              If none are given, the built-in DEFAULT_TICKETS corpus runs.
#   --config:  gh-optivem.yaml variant to exercise (forwarded as-is to every
#              rehearsal). Default: DEFAULT_CONFIG below.
#
# Examples:
#   # Full built-in corpus (61 65 68 69 70 71 72 76), default config:
#   bash atdd-rehearsal-loop.sh
#
#   # A single ticket:
#   bash atdd-rehearsal-loop.sh 68
#
#   # A few specific tickets, in the given order:
#   bash atdd-rehearsal-loop.sh 68 69 76
#
#   # Override the config for whichever tickets you pass:
#   bash atdd-rehearsal-loop.sh --config gh-optivem-monolith-typescript.yaml 65
#
#   # Override the config for the full default corpus:
#   bash atdd-rehearsal-loop.sh --config gh-optivem-multitier-java.yaml
#
# Autonomous by contract:
#   - Every rehearsal is invoked with --auto --headless (fully unattended).
#   - REHEARSAL_CLEANUP=yes is exported so each rehearsal deletes its worktree
#     + branch on exit WITHOUT prompting. The per-run .log file is always kept
#     (atdd-rehearsal.sh writes it as a sibling of the worktree), so a torn-down
#     run still leaves a postmortem record under <academy>/worktrees/.
#   - stdin is redirected from /dev/null per run so no stray read can block.
#
# Failure policy: STOP on the first ticket whose rehearsal exits non-zero. The
# summary printed so far tells you what passed; re-run with just the failing
# ticket (and drop --headless / REHEARSAL_CLEANUP to inspect) to debug it.

# === LOOP CONFIG === (edit these for your setup)
DEFAULT_CONFIG="gh-optivem-monolith-java.yaml"
# The CONTRIBUTING.md rehearsal corpus, in document order. Only the leading
# issue number is data — the loop forwards it to atdd-rehearsal.sh as-is; the
# trailing comment (title · clickable issue URL · what the story exercises) is
# documentation carried over from CONTRIBUTING.md so this list is self-describing.
DEFAULT_TICKETS=(
  # --- structural change (UI redesign) ---
  61  # Redesigning New Order UI                  https://github.com/optivem/shop/issues/61
      #   Structural change — reworks the New Order UI layout.

  # --- behavioral change (user stories) ---
  65  # View product list                         https://github.com/optivem/shop/issues/65
      #   Read-only flow.
  68  # Apply automatic quantity discount on cart lines   https://github.com/optivem/shop/issues/68
      #   Write flow with a calculation rule. Discount fields already exist on
      #   ViewOrderResponse, so no system driver-port change.
  69  # Reject order with line quantity of 100     https://github.com/optivem/shop/issues/69
      #   Write flow with a validation rule. No external system.
  70  # Return a delivered order                   https://github.com/optivem/shop/issues/70
      #   Write flow extending the DSL + driver surface. No external system.
  71  # Gift-wrap an order                         https://github.com/optivem/shop/issues/71
      #   Write flow adding a new field to the existing DSL. No external system.
  72  # Charge shipping based on product weight from ERP  https://github.com/optivem/shop/issues/72
      #   THE FULL-BPMN STORY — the only one that trips all three change-detection
      #   gates (at-dsl-port-changed, at-external-driver-port-changed,
      #   at-system-driver-port-changed) and walks the entire flow end-to-end.
      #   ERP is real-kind: simulator, so it also takes the longest external
      #   branch (verify-fail real → author real sim → verify-pass real →
      #   stub red→green).

  # --- bug fix (reproduce then fix) ---
  76  # Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00  https://github.com/optivem/shop/issues/76
      #   The only bug-fix rehearsal: a failing acceptance test reproduces an
      #   EXISTING defect (blackout blocks only 22:00–22:30 vs the documented
      #   22:00–23:00), then a pure behavioral fix (extend window to 23:00)
      #   turns it green. No DSL or driver-port change.
)
# === END LOOP CONFIG ===

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REHEARSAL="$SCRIPT_DIR/atdd-rehearsal.sh"

if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'; C_RESET=$'\033[0m'
else
  C_BOLD=""; C_RESET=""
fi
log() { echo "${C_BOLD}[loop]${C_RESET} $*"; }

CONFIG="$DEFAULT_CONFIG"
TICKETS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      sed -n '9,49p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    -c|--config)
      [[ $# -ge 2 ]] || { echo "ERROR: $1 requires a value" >&2; exit 2; }
      CONFIG="$2"; shift 2 ;;
    --config=*)
      CONFIG="${1#--config=}"; shift ;;
    --)
      shift; while [[ $# -gt 0 ]]; do TICKETS+=("$1"); shift; done ;;
    -*)
      echo "ERROR: unknown flag: $1" >&2; exit 2 ;;
    *)
      TICKETS+=("$1"); shift ;;
  esac
done

if [[ ${#TICKETS[@]} -eq 0 ]]; then
  TICKETS=("${DEFAULT_TICKETS[@]}")
fi

if [[ ! -x "$REHEARSAL" && ! -f "$REHEARSAL" ]]; then
  echo "ERROR: rehearsal script not found at $REHEARSAL" >&2
  exit 2
fi

log "Tickets: ${TICKETS[*]}"
log "Config:  $CONFIG"
log "Mode:    --auto --headless, auto-delete worktrees (logs kept), stop-on-failure"
echo ""

# Each iteration appends "<ticket> <PASS|FAIL>"; printed as a table at the end.
RESULTS=()
print_summary() {
  echo ""
  log "${C_BOLD}Summary${C_RESET}"
  for r in "${RESULTS[@]}"; do
    log "  $r"
  done
}

for ticket in "${TICKETS[@]}"; do
  log "${C_BOLD}=== Rehearsing #${ticket} ===${C_RESET}"
  # Auto-delete the worktree, never prompt, never read stdin.
  if REHEARSAL_CLEANUP=yes bash "$REHEARSAL" "$ticket" \
        --config "$CONFIG" --auto --headless </dev/null; then
    RESULTS+=("#${ticket}  PASS")
    log "#${ticket} PASS"
  else
    rc=$?
    RESULTS+=("#${ticket}  FAIL (exit $rc)")
    log "#${ticket} FAILED (exit $rc) — stopping per failure policy."
    print_summary
    exit "$rc"
  fi
done

print_summary
log "All ${#TICKETS[@]} ticket(s) passed."

} && exit
