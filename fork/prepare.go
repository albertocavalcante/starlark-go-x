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
)

type prepareReport struct {
	Status    string `json:"status"`
	Base      string `json:"base"`
	Upstream  string `json:"upstream"`
	Candidate string `json:"candidate,omitempty"`
	Tree      string `json:"tree,omitempty"`
	Patch     string `json:"patch,omitempty"`
	Error     string `json:"error,omitempty"`
}

func resolve(repo, revision string) (string, error) {
	b, err := git(repo, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	return strings.TrimSpace(string(b)), err
}

func requireRevision(repo, revision, expected string) error {
	actual, err := resolve(repo, revision)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("source revision changed from %s to %s; prepare again", expected, actual)
	}
	return nil
}

func outputPath(repo, output string) (string, error) {
	if output == "" {
		return "", fmt.Errorf("an explicit new output directory is required")
	}
	abs, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	protected := []string{repo}
	for _, arg := range []string{"--absolute-git-dir", "--git-common-dir"} {
		b, err := git(repo, nil, "rev-parse", "--path-format=absolute", arg)
		if err != nil {
			return "", err
		}
		protected = append(protected, strings.TrimSpace(string(b)))
	}
	// Compare actual directory identities, including case-insensitive filesystems
	// and symlink aliases, rather than relying on lexical path prefixes.
	for _, path := range protected {
		root, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		for current := parent; ; current = filepath.Dir(current) {
			info, err := os.Stat(current)
			if err != nil {
				return "", err
			}
			if os.SameFile(root, info) {
				return "", fmt.Errorf("output must be outside the source checkout and Git metadata")
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".prepare-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// prepare performs all writes in a new disposable repository. It never fetches
// from the network or publishes refs. A failed replay keeps its index and report.
func prepare(repo, revision, target, output string) (r prepareReport, err error) {
	r.Status, r.Upstream = "failed", target
	if !fullHex(target, 20) {
		return r, fmt.Errorf("upstream must be a full lowercase commit SHA")
	}
	repo, err = filepath.Abs(repo)
	if err != nil {
		return r, err
	}
	r.Base, err = resolve(repo, revision)
	if err != nil {
		return r, err
	}
	r.Tree, err = verify(repo, r.Base)
	if err != nil {
		return r, err
	}
	b, err := git(repo, nil, "show", r.Base+":fork/manifest.json")
	if err != nil {
		return r, err
	}
	var m manifest
	// verify has already strictly decoded and validated these immutable bytes.
	if err := json.Unmarshal(b, &m); err != nil {
		return r, err
	}
	if m.Upstream.Commit == target {
		r.Status = "noop"
		if err := requireRevision(repo, revision, r.Base); err != nil {
			r.Status = "stale"
			return r, err
		}
		return r, nil
	}
	archive := "refs/tags/upstream/" + target
	actual, err := resolve(repo, archive)
	if err != nil || actual != target {
		return r, fmt.Errorf("fetch and verify the exact target archive first: %s (%v)", archive, err)
	}
	if _, err := git(repo, nil, "merge-base", "--is-ancestor", m.Upstream.Commit, target); err != nil {
		return r, fmt.Errorf("target is not a forward upstream update: %w", err)
	}
	output, err = outputPath(repo, output)
	if err != nil {
		return r, err
	}
	if err := os.Mkdir(output, 0755); err != nil {
		return r, err
	}
	r.Status, r.Tree = "preparing", ""
	defer func() {
		if err != nil {
			r.Error = err.Error()
			if r.Status == "preparing" || r.Status == "prepared" {
				r.Status = "failed"
			}
		}
		if saveErr := writeJSON(filepath.Join(output, "report.json"), r); saveErr != nil {
			r.Status = "failed"
			err = fmt.Errorf("record preparation report: %v (prior error: %v)", saveErr, err)
		}
	}()
	if err := writeJSON(filepath.Join(output, "report.json"), r); err != nil {
		return r, err
	}
	work := filepath.Join(output, "repo")
	if err := os.Mkdir(work, 0755); err != nil {
		return r, err
	}
	run := func(input []byte, args ...string) ([]byte, error) { return git(work, input, args...) }
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "starlark-go-x replay"},
		{"config", "user.email", "replay@localhost"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := run(nil, args...); err != nil {
			return r, err
		}
	}
	if err := fetchInputs(work, repo, r.Base, m.Upstream); err != nil {
		return r, err
	}
	if _, err := run(nil, "fetch", "--no-tags", "--", repo, archive+":"+archive); err != nil {
		return r, err
	}
	if actual, err := resolve(work, archive); err != nil || actual != target {
		return r, fmt.Errorf("fetched target archive identity mismatch (%v)", err)
	}
	if _, err := run(nil, "update-ref", m.Upstream.ArchiveRef, m.Upstream.Commit); err != nil {
		return r, err
	}
	paths, err := run(nil, "ls-tree", "--name-only", target, "--", "fork")
	if err != nil || len(paths) != 0 {
		return r, fmt.Errorf("target owns fork/ or cannot be inspected: %v", err)
	}
	patches := make([][]byte, len(m.Patches))
	for i, p := range m.Patches {
		patches[i], err = run(nil, "show", r.Base+":fork/patches/"+p.File)
		if err != nil {
			return r, err
		}
	}
	checkpoint := func(parent, id string) (string, error) {
		tree, err := run(nil, "write-tree")
		if err != nil {
			return "", err
		}
		b, err := run([]byte("chore(fork): replay "+id+"\n"), "commit-tree", strings.TrimSpace(string(tree)), "-p", parent)
		return strings.TrimSpace(string(b)), err
	}
	// Reconstruct the old queue first to seed the blobs needed by three-way replay
	// and a real range-diff. These disposable commits are never published.
	oldHead := m.Upstream.Commit
	if _, err := run(nil, "checkout", "-q", "--detach", oldHead); err != nil {
		return r, err
	}
	for i, p := range patches {
		if _, err := run(p, "apply", "--index", "--binary", "--whitespace=nowarn", "-"); err != nil {
			return r, err
		}
		oldHead, err = checkpoint(oldHead, m.Patches[i].ID)
		if err != nil {
			return r, err
		}
	}
	if _, err := run(nil, "reset", "--hard", target); err != nil {
		return r, err
	}
	newHead := target
	for i, p := range patches {
		before, err := run(nil, "write-tree")
		if err != nil {
			return r, err
		}
		if _, err := run(p, "apply", "--3way", "--index", "--binary", "--whitespace=nowarn", "-"); err != nil {
			r.Status, r.Patch = "conflict", m.Patches[i].ID
			return r, fmt.Errorf("patch %s needs manual resolution in %s: %w", r.Patch, work, err)
		}
		patches[i], err = run(nil, "diff", "--cached", "--binary", "--full-index", strings.TrimSpace(string(before)))
		if err != nil {
			return r, err
		}
		if len(patches[i]) == 0 {
			r.Patch = m.Patches[i].ID
			return r, fmt.Errorf("patch %s became empty; prove equivalence before retiring it", r.Patch)
		}
		newHead, err = checkpoint(newHead, m.Patches[i].ID)
		if err != nil {
			return r, err
		}
	}
	rangeDiff, err := run(nil, "range-diff", "--no-color", m.Upstream.Commit+".."+oldHead, target+".."+newHead)
	if err != nil {
		return r, err
	}
	if err := os.WriteFile(filepath.Join(output, "range.diff"), rangeDiff, 0644); err != nil {
		return r, err
	}
	if _, err := run(nil, "read-tree", "-u", "--prefix=fork/", r.Base+":fork"); err != nil {
		return r, err
	}
	m.Upstream.Commit, m.Upstream.ArchiveRef = target, archive
	for i, p := range m.Patches {
		if err := os.WriteFile(filepath.Join(work, "fork", "patches", p.File), patches[i], 0644); err != nil {
			return r, err
		}
		m.Patches[i].SHA256 = fmt.Sprintf("%x", sha256.Sum256(patches[i]))
	}
	if err := writeJSON(filepath.Join(work, "fork", "manifest.json"), m); err != nil {
		return r, err
	}
	if _, err := run(nil, "add", "--", "fork"); err != nil {
		return r, err
	}
	r.Candidate, err = checkpoint(r.Base, "upstream-"+target[:12])
	if err != nil {
		return r, err
	}
	if _, err := run(nil, "reset", "--hard", r.Candidate); err != nil {
		return r, err
	}
	r.Tree, err = verify(work, r.Candidate)
	if err != nil {
		return r, err
	}
	if err := requireRevision(repo, revision, r.Base); err != nil {
		r.Status = "stale"
		return r, err
	}
	r.Status = "prepared"
	return r, nil
}
