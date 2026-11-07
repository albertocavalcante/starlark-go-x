// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nextUpstream(t *testing.T, repo string, m manifest, conflict bool) string {
	t.Helper()
	head := command(t, repo, "rev-parse", "HEAD")
	// The shared fixture records an executable index mode independently of the
	// host filesystem. Normalize that throwaway checkout before changing bases.
	command(t, repo, "checkout", "-q", "-f", "--detach", m.Upstream.Commit)
	name := "upstream-new"
	if conflict {
		name = "value"
	}
	put(t, repo, name, []byte("upstream update\n"))
	command(t, repo, "add", name)
	command(t, repo, "commit", "-qm", "upstream update")
	next := command(t, repo, "rev-parse", "HEAD")
	command(t, repo, "tag", "upstream/"+next)
	command(t, repo, "checkout", "-q", "--detach", head)
	return next
}

func TestPrepareSnapshot(t *testing.T) {
	repo, m := fixture(t)
	next := nextUpstream(t, repo, m, false)
	put(t, repo, "value", []byte("local draft\n"))
	put(t, repo, "untracked", []byte("private draft\n"))
	before := command(t, repo, "rev-parse", "HEAD")
	status := command(t, repo, "status", "--porcelain")
	out := filepath.Join(t.TempDir(), "candidate")
	r, err := prepare(repo, "HEAD", next, out)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "prepared" || r.Base != before || r.Upstream != next {
		t.Fatalf("wrong report: %+v", r)
	}
	candidate := filepath.Join(out, "repo")
	if parent := command(t, candidate, "rev-parse", "HEAD^"); parent != before {
		t.Fatalf("candidate parent = %s, want captured fork head", parent)
	}
	if tree, err := verify(candidate, "HEAD"); err != nil || tree != r.Tree {
		t.Fatalf("candidate failed exact verification: %s %v", tree, err)
	}
	if got := command(t, candidate, "show", "HEAD:value"); got != "fork" {
		t.Fatalf("fork change lost: %q", got)
	}
	if got := command(t, candidate, "show", "HEAD:upstream-new"); got != "upstream update" {
		t.Fatalf("upstream change lost: %q", got)
	}
	if command(t, repo, "rev-parse", "HEAD") != before || command(t, repo, "status", "--porcelain") != status {
		t.Fatal("source checkout changed")
	}
	// Refreshed payload and metadata trees are stable across repeated preparation;
	// disposable working commit IDs and report bytes need not be identical.
	other, err := prepare(repo, "HEAD", next, filepath.Join(t.TempDir(), "other"))
	if err != nil || other.Tree != r.Tree {
		t.Fatalf("preparation did not reproduce the candidate tree: %+v %v", other, err)
	}
}

func TestPrepareConflictPreservesEvidence(t *testing.T) {
	repo, m := fixture(t)
	next := nextUpstream(t, repo, m, true)
	before := command(t, repo, "rev-parse", "HEAD")
	out := filepath.Join(t.TempDir(), "conflict")
	r, err := prepare(repo, "HEAD", next, out)
	if err == nil || r.Status != "conflict" || r.Patch != "fixture" {
		t.Fatalf("conflict accepted or wrong report: %+v %v", r, err)
	}
	if command(t, filepath.Join(out, "repo"), "ls-files", "--unmerged") == "" {
		t.Fatal("conflicted index was not retained")
	}
	b, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved prepareReport
	if err := json.Unmarshal(b, &saved); err != nil || saved.Status != "conflict" {
		t.Fatalf("missing conflict report: %s %v", b, err)
	}
	if command(t, repo, "rev-parse", "HEAD") != before {
		t.Fatal("source ref changed on conflict")
	}
	// Reusing an interrupted/conflicted output never overwrites recovery evidence.
	if _, err := prepare(repo, "HEAD", next, out); err == nil {
		t.Fatal("existing conflict workspace overwritten")
	}
}

func TestPrepareNoopAndInvalidInputs(t *testing.T) {
	repo, m := fixture(t)
	out := filepath.Join(t.TempDir(), "noop")
	r, err := prepare(repo, "HEAD", m.Upstream.Commit, out)
	if err != nil || r.Status != "noop" {
		t.Fatalf("no-op failed: %+v %v", r, err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("no-op created output")
	}
	next := nextUpstream(t, repo, m, false)
	for _, target := range []string{"HEAD", strings.Repeat("0", 40)} {
		if _, err := prepare(repo, "HEAD", target, out); err == nil {
			t.Fatalf("invalid/unarchived upstream %q accepted", target)
		}
	}
	if _, err := prepare(repo, "HEAD", next, filepath.Join(repo, "nested")); err == nil {
		t.Fatal("output inside source accepted")
	}
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.Mkdir(occupied, 0755); err != nil {
		t.Fatal(err)
	}
	put(t, occupied, "sentinel", []byte("keep\n"))
	if _, err := prepare(repo, "HEAD", next, occupied); err == nil {
		t.Fatal("existing output accepted")
	}
	if b, err := os.ReadFile(filepath.Join(occupied, "sentinel")); err != nil || string(b) != "keep\n" {
		t.Fatal("existing output was changed")
	}
}

func TestPrepareProtectsSharedGitDirectory(t *testing.T) {
	repo, m := fixture(t)
	next := nextUpstream(t, repo, m, false)
	linked := filepath.Join(t.TempDir(), "linked")
	command(t, repo, "worktree", "add", "--detach", linked, "HEAD")
	if _, err := prepare(linked, "HEAD", next, filepath.Join(repo, ".git", "unsafe")); err == nil {
		t.Fatal("output inside the shared Git directory accepted")
	}
}

func TestPreparationRefFreshness(t *testing.T) {
	repo, _ := fixture(t)
	before := command(t, repo, "rev-parse", "HEAD")
	command(t, repo, "commit", "--allow-empty", "-qm", "advance")
	if err := requireRevision(repo, "HEAD", before); err == nil {
		t.Fatal("changed input ref accepted")
	}
}

func TestGitRejectsNetworkTransport(t *testing.T) {
	_, err := git(t.TempDir(), nil, "ls-remote", "https://127.0.0.1:1/forbidden")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("network was not rejected before a connection: %v", err)
	}
}

func TestPrepareRejectsInterruptedOutput(t *testing.T) {
	repo, m := fixture(t)
	next := nextUpstream(t, repo, m, false)
	out := filepath.Join(t.TempDir(), "interrupted")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "report.json")
	if err := writeJSON(path, prepareReport{Status: "preparing"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := prepare(repo, "HEAD", next, out); err == nil {
		t.Fatal("interrupted output reused")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("interrupted report overwritten")
	}
}

func TestPrepareRequiresReviewBeforeRetiringPatch(t *testing.T) {
	repo, m := fixture(t)
	command(t, repo, "checkout", "-f", "HEAD")
	command(t, repo, "restore", "--source="+m.Upstream.Commit, "--staged", "--worktree", "--", "binary", "gone")
	p := []byte(command(t, repo, "diff", "--binary", "--full-index", m.Upstream.Commit, "--", "value") + "\n")
	put(t, repo, "fork/patches/001.patch", p)
	m.Patches[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256(p))
	saveManifest(t, repo, m)
	command(t, repo, "add", "fork")
	command(t, repo, "commit", "-qm", "single change fixture")
	before := command(t, repo, "rev-parse", "HEAD")
	command(t, repo, "checkout", "-q", "--detach", m.Upstream.Commit)
	command(t, repo, "restore", "--source="+before, "--staged", "--worktree", "--", "value")
	command(t, repo, "commit", "-qm", "upstream supplies the fork change")
	next := command(t, repo, "rev-parse", "HEAD")
	command(t, repo, "tag", "upstream/"+next)
	command(t, repo, "checkout", "-q", "--detach", before)
	r, err := prepare(repo, "HEAD", next, filepath.Join(t.TempDir(), "empty"))
	if err == nil || !strings.Contains(err.Error(), "became empty") || r.Status == "prepared" {
		t.Fatalf("absorbed patch retired without review: %+v %v", r, err)
	}
}
