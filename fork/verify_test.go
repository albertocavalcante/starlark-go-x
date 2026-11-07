// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func command(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull}, args...)...)
	c.Dir = dir
	c.Env = gitEnv()
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

func put(t *testing.T, repo, name string, data []byte) {
	t.Helper()
	p := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func saveManifest(t *testing.T, repo string, m manifest) {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	put(t, repo, "fork/manifest.json", append(b, '\n'))
}

// The fixture exercises bytes, deletion and index modes without network access.
func fixture(t *testing.T) (string, manifest) {
	t.Helper()
	repo := t.TempDir()
	command(t, repo, "init", "-q")
	put(t, repo, "value", []byte("upstream\n"))
	put(t, repo, "gone", []byte("remove me\n"))
	command(t, repo, "add", "value", "gone")
	command(t, repo, "commit", "-qm", "upstream")
	base := command(t, repo, "rev-parse", "HEAD")
	archive := "refs/tags/upstream/" + base
	command(t, repo, "tag", strings.TrimPrefix(archive, "refs/tags/"))
	put(t, repo, "value", []byte("fork\n"))
	put(t, repo, "binary", []byte{0, 1, 2, 255})
	command(t, repo, "rm", "gone")
	command(t, repo, "add", "value", "binary")
	command(t, repo, "update-index", "--chmod=+x", "value")
	command(t, repo, "commit", "-qm", "fork")
	patchBytes := []byte(command(t, repo, "diff", "--binary", "--full-index", base, "HEAD") + "\n")
	m := manifest{
		Version:  1,
		Upstream: upstream{URL: upstreamURL, Branch: "master", Commit: base, ArchiveRef: archive},
		Patches:  []patch{{ID: "fixture", File: "001.patch", SHA256: fmt.Sprintf("%x", sha256.Sum256(patchBytes)), Reason: "fixture", Tests: []string{"fixture"}, Disposition: "fork-only"}},
	}
	put(t, repo, "fork/patches/001.patch", patchBytes)
	saveManifest(t, repo, m)
	command(t, repo, "add", "fork")
	command(t, repo, "commit", "-qm", "metadata")
	return repo, m
}

func TestReconstruct(t *testing.T) {
	repo, _ := fixture(t)
	if _, err := verify(repo, "HEAD"); err != nil {
		t.Fatal(err)
	}
	// Verification must not depend on an object's presence in another checkout.
	clone := filepath.Join(t.TempDir(), "clone")
	command(t, t.TempDir(), "clone", "--no-local", "--no-tags", repo, clone)
	if _, err := verify(clone, "HEAD"); err == nil {
		t.Fatal("missing archive accepted")
	}
	command(t, clone, "fetch", "origin", "refs/tags/upstream/*:refs/tags/upstream/*")
	if _, err := verify(clone, "HEAD"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectChangedInputs(t *testing.T) {
	for _, scenario := range []string{"source", "hash", "unlisted", "archive", "metadata-patch"} {
		t.Run(scenario, func(t *testing.T) {
			repo, m := fixture(t)
			switch scenario {
			case "source":
				put(t, repo, "value", []byte("unrecorded\n"))
				command(t, repo, "add", "value")
			case "hash":
				m.Patches[0].SHA256 = strings.Repeat("0", 64)
			case "unlisted":
				put(t, repo, "fork/patches/unused.patch", []byte("unused"))
			case "archive":
				command(t, repo, "tag", "-f", strings.TrimPrefix(m.Upstream.ArchiveRef, "refs/tags/"), "HEAD")
			case "metadata-patch":
				p := []byte("diff --git a/fork/escape b/fork/escape\nnew file mode 100644\n--- /dev/null\n+++ b/fork/escape\n@@ -0,0 +1 @@\n+escape\n")
				put(t, repo, "fork/patches/001.patch", p)
				m.Patches[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256(p))
			}
			saveManifest(t, repo, m)
			command(t, repo, "add", "fork")
			command(t, repo, "commit", "--allow-empty", "-qm", "tamper")
			if _, err := verify(repo, "HEAD"); err == nil {
				t.Fatal("changed input accepted")
			}
		})
	}
}

func TestManifestValidation(t *testing.T) {
	_, good := fixture(t)
	for _, scenario := range []string{"version", "url", "branch", "commit", "ref", "path", "duplicate", "dependency", "reason", "tests", "disposition", "hash", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			b, _ := json.Marshal(good)
			var m manifest
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "version":
				m.Version = 2
			case "url":
				m.Upstream.URL = "https://example.invalid/repo"
			case "branch":
				m.Upstream.Branch = "other"
			case "commit":
				m.Upstream.Commit = "HEAD"
			case "ref":
				m.Upstream.ArchiveRef = "refs/heads/master"
			case "path":
				m.Patches[0].File = "../escape.patch"
			case "duplicate":
				m.Patches = append(m.Patches, m.Patches[0])
			case "dependency":
				m.Patches[0].Dependencies = []string{"later"}
			case "reason":
				m.Patches[0].Reason = ""
			case "tests":
				m.Patches[0].Tests = nil
			case "disposition":
				m.Patches[0].Disposition = ""
			case "hash":
				m.Patches[0].SHA256 = "oops"
			case "empty":
				m.Patches = nil
			}
			if err := m.validate(); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}
