#!/usr/bin/env bash
set -euo pipefail

# Renders every Mermaid fence in docs/process-diagram.md to a standalone
# SVG under docs/images/, naming each file after its section heading
# (e.g. process-diagram-2-at-cycle.svg). Useful for previewing in tools
# that don't render Mermaid natively, or for embedding in external
# docs/slides.
#
# Requires: npx (Node.js). The mermaid-cli package is fetched on demand.
#
# Usage:
#     bash scripts/render-svgs.sh

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$repo_root/docs/process-diagram.md"
out_dir="$repo_root/docs/images"

mkdir -p "$out_dir"
rm -f "$out_dir"/process-diagram-*.svg

# Pin mermaid-cli so local renders match what the regenerate-diagram
# workflow commits. Bump in lockstep with .github/workflows/regenerate-diagram.yml.
npx -y -p @mermaid-js/mermaid-cli@11.14.0 mmdc -i "$src" -o "$out_dir/process-diagram.svg"

# Rename mmdc's index-numbered outputs to include the section heading
# slug. The Nth `## ` heading in the source markdown corresponds to
# process-diagram-N.svg (mmdc renders fences in document order).
mapfile -t slugs < <(
    grep -E '^## ' "$src" \
        | sed -E 's/^## //' \
        | tr '[:upper:]' '[:lower:]' \
        | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g'
)

for i in "${!slugs[@]}"; do
    n=$((i + 1))
    from="$out_dir/process-diagram-${n}.svg"
    to="$out_dir/process-diagram-${n}-${slugs[$i]}.svg"
    if [[ -f "$from" ]]; then
        mv "$from" "$to"
    fi
done

echo "Rendered SVGs to $out_dir"
