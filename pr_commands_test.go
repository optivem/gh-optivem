package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPrMergeBuildArgs_DefaultSquash(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--squash"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_ExplicitSquash(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{Squash: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--squash"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit-squash args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_Rebase(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{Rebase: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--rebase"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rebase args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_SquashAndRebaseRejected(t *testing.T) {
	_, err := buildPrMergeArgs(prMergeOptions{Squash: true, Rebase: true}, nil)
	if err == nil {
		t.Fatalf("expected error when both --squash and --rebase are set, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error message should mention mutual exclusion, got: %v", err)
	}
}

func TestPrMergeBuildArgs_PositionalPRNumber(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{}, []string{"123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--squash", "123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positional PR-number args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_RebaseWithPRNumber(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{Rebase: true}, []string{"42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--rebase", "42"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rebase+PR-number args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_DeleteBranchAndAuto(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{Auto: true, DeleteBranch: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--squash", "--auto", "--delete-branch"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auto+delete-branch args mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestPrMergeBuildArgs_AllFlagsWithPRNumber(t *testing.T) {
	got, err := buildPrMergeArgs(prMergeOptions{Rebase: true, Auto: true, DeleteBranch: true}, []string{"7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pr", "merge", "--rebase", "--auto", "--delete-branch", "7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("all-flags args mismatch:\n got=%v\nwant=%v", got, want)
	}
}
