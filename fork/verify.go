// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command fork verifies a committed revision against its recorded upstream and
// patch queue. It never updates the source repository or uses the network.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const upstreamURL = "https://github.com/google/starlark-go.git"

type upstream struct {
	URL        string `json:"url"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	ArchiveRef string `json:"archive_ref"`
}

type patch struct {
	ID           string   `json:"id"`
	File         string   `json:"file"`
	SHA256       string   `json:"sha256"`
	Reason       string   `json:"reason"`
	Dependencies []string `json:"dependencies"`
	Tests        []string `json:"tests"`
	Disposition  string   `json:"upstream_disposition"`
}

type manifest struct {
	Version  int      `json:"version"`
	Upstream upstream `json:"upstream"`
	Patches  []patch  `json:"patches"`
}

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func fullHex(s string, size int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == size && s == strings.ToLower(s)
}

func (m manifest) validate() error {
	if m.Version != 1 || m.Upstream.URL != upstreamURL || m.Upstream.Branch != "master" || !fullHex(m.Upstream.Commit, 20) || m.Upstream.ArchiveRef != "refs/tags/upstream/"+m.Upstream.Commit {
		return fmt.Errorf("unsupported manifest version or upstream identity")
	}
	if len(m.Patches) == 0 {
		return fmt.Errorf("patch queue is empty")
	}
	ids, files := map[string]bool{}, map[string]bool{}
	for _, p := range m.Patches {
		if !safeName.MatchString(p.ID) || !safeName.MatchString(p.File) || !strings.HasSuffix(p.File, ".patch") || ids[p.ID] || files[p.File] || !fullHex(p.SHA256, 32) || strings.TrimSpace(p.Reason) == "" || strings.TrimSpace(p.Disposition) == "" || len(p.Tests) == 0 {
			return fmt.Errorf("invalid or duplicate patch %q", p.ID)
		}
		for _, test := range p.Tests {
			if strings.TrimSpace(test) == "" {
				return fmt.Errorf("empty test for patch %q", p.ID)
			}
		}
		deps := map[string]bool{}
		for _, dep := range p.Dependencies {
			if !ids[dep] || deps[dep] {
				return fmt.Errorf("patch %q has missing, repeated or out-of-order dependency %q", p.ID, dep)
			}
			deps[dep] = true
		}
		ids[p.ID], files[p.File] = true, true
	}
	return nil
}

// Ignore inherited Git routing/configuration, including injected config entries.
// Commands use a fresh repository with no hooks, filters or conflict reuse.
func gitEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
}

func git(repo string, input []byte, args ...string) ([]byte, error) {
	settings := []string{"-c", "core.hooksPath=" + os.DevNull, "-c", "core.autocrlf=false", "-c", "rerere.enabled=false"}
	c := exec.Command("git", append(settings, args...)...)
	c.Dir, c.Env = repo, gitEnv()
	c.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, &stderr)
	}
	return b, nil
}

// Validate the fetched ref too: the source ref may move after the initial check.
func fetchInputs(tmp, repo, head string, u upstream) error {
	const fetched = "refs/tags/recorded-upstream"
	if _, err := git(tmp, nil, "fetch", "--no-tags", "--", repo, head, u.ArchiveRef+":"+fetched); err != nil {
		return err
	}
	b, err := git(tmp, nil, "rev-parse", "--verify", fetched+"^{commit}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) != u.Commit {
		return fmt.Errorf("fetched upstream archive identity mismatch")
	}
	return nil
}

func verify(repo, revision string) (string, error) {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	resolved, err := git(repo, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(string(resolved))
	read := func(path string) ([]byte, error) { return git(repo, nil, "show", head+":"+path) }
	b, err := read("fork/manifest.json")
	if err != nil {
		return "", err
	}
	var m manifest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return "", err
	}
	if err := d.Decode(new(interface{})); err != io.EOF {
		return "", fmt.Errorf("manifest contains trailing JSON")
	}
	if err := m.validate(); err != nil {
		return "", err
	}
	listing, err := git(repo, nil, "ls-tree", "-rz", "--name-only", head, "--", "fork/patches/")
	if err != nil {
		return "", err
	}
	wanted := map[string]bool{}
	patches := make([][]byte, len(m.Patches))
	for i, p := range m.Patches {
		path := "fork/patches/" + p.File
		wanted[path] = true
		patches[i], err = read(path)
		if err != nil {
			return "", err
		}
		if fmt.Sprintf("%x", sha256.Sum256(patches[i])) != p.SHA256 {
			return "", fmt.Errorf("patch %q checksum mismatch", p.ID)
		}
	}
	for _, path := range strings.Split(strings.TrimSuffix(string(listing), "\x00"), "\x00") {
		if !wanted[path] {
			return "", fmt.Errorf("unlisted patch entry %q", path)
		}
	}
	archive, err := git(repo, nil, "rev-parse", "--verify", m.Upstream.ArchiveRef+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("fetch the recorded archive ref first: %w", err)
	}
	if strings.TrimSpace(string(archive)) != m.Upstream.Commit {
		return "", fmt.Errorf("upstream archive identity mismatch")
	}
	tmp, err := os.MkdirTemp("", "starlark-fork-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	run := func(input []byte, args ...string) ([]byte, error) { return git(tmp, input, args...) }
	if _, err := run(nil, "init", "-q"); err != nil {
		return "", err
	}
	if err := fetchInputs(tmp, repo, head, m.Upstream); err != nil {
		return "", err
	}
	paths, err := run(nil, "ls-tree", "--name-only", m.Upstream.Commit, "--", "fork")
	if err != nil {
		return "", err
	}
	if len(paths) != 0 {
		return "", fmt.Errorf("upstream now owns fork/: namespace decision required")
	}
	if _, err := run(nil, "checkout", "-q", "--detach", m.Upstream.Commit); err != nil {
		return "", err
	}
	for i, p := range patches {
		if _, err := run(p, "apply", "--index", "--binary", "--whitespace=nowarn", "-"); err != nil {
			return "", fmt.Errorf("patch %q: %w", m.Patches[i].ID, err)
		}
		paths, err := run(nil, "ls-files", "--", "fork")
		if err != nil || len(paths) != 0 {
			return "", fmt.Errorf("patch %q alters reserved fork/ metadata (git error: %v)", m.Patches[i].ID, err)
		}
	}
	if _, err := run(nil, "read-tree", "--prefix=fork/", head+":fork"); err != nil {
		return "", err
	}
	tree, err := run(nil, "write-tree")
	if err != nil {
		return "", err
	}
	expected, err := run(nil, "rev-parse", head+"^{tree}")
	if err != nil {
		return "", err
	}
	if !bytes.Equal(tree, expected) {
		return "", fmt.Errorf("reconstructed tree %s differs from revision tree %s", strings.TrimSpace(string(tree)), strings.TrimSpace(string(expected)))
	}
	return strings.TrimSpace(string(tree)), nil
}

func main() {
	repo := flag.String("repo", ".", "local source repository (unchanged by verification)")
	revision := flag.String("revision", "HEAD", "committed revision to reconstruct; dirty edits are excluded")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	tree, err := verify(*repo, *revision)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("verified tree", tree)
}
