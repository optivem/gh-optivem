// Package cmdctx threads the resolved auto-approve policy from the root
// command's PersistentPreRunE down to every Cobra subcommand's Run via a
// typed context key. Pure plumbing; the policy itself lives in
// internal/approval.
//
// Why a separate package: keeping the context-key type unexported here and
// exposing only With/From accessors prevents accidental cross-package
// shadow keys, and lets the approval package stay focused on policy
// resolution without leaking Cobra-shaped helpers.
package cmdctx

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/approval"
)

type approvalKey struct{}

// WithApproval returns a child context carrying r. main.go's
// PersistentPreRunE calls this once at startup and SetContext's the
// result onto the executing command so every descendant Run sees the
// same Resolved.
func WithApproval(ctx context.Context, r approval.Resolved) context.Context {
	return context.WithValue(ctx, approvalKey{}, r)
}

// ApprovalFromContext returns the Resolved stashed by WithApproval. A
// context without one yields the zero value — Auto=false, no
// ConfirmSet entries — which is the safe cautious default (every
// approval.Confirm call falls through to the interactive prompt).
func ApprovalFromContext(ctx context.Context) approval.Resolved {
	if ctx == nil {
		return approval.Resolved{}
	}
	if r, ok := ctx.Value(approvalKey{}).(approval.Resolved); ok {
		return r
	}
	return approval.Resolved{}
}

// Approval is the Cobra-shaped sugar: read the Resolved off the
// command's own context. Equivalent to ApprovalFromContext(cmd.Context())
// but reads more naturally at the call site of a Run function.
func Approval(cmd *cobra.Command) approval.Resolved {
	if cmd == nil {
		return approval.Resolved{}
	}
	return ApprovalFromContext(cmd.Context())
}
