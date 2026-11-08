# Maintaining starlark-go-x

Every published revision records one Google upstream commit and an ordered queue
of intentional fork changes in `manifest.json`. Alberto Cavalcante owns his fork
additions; upstream notices and the BSD license remain intact. See `FORK_NOTICE`.

The [pointer diagnostic assessment](VET.md) records the remaining unpack
optimization warning and the evidence required to review it. Raw full-package
vet remains visibly non-green while that implementation remains unchanged.

## Verify a revision

From a clean checkout at the revision being checked:

```sh
git fetch origin 'refs/tags/upstream/*:refs/tags/upstream/*'
go test ./fork
go run ./fork -revision HEAD
internal/test.sh
```

The verifier reads **committed objects**, excluding dirty and staged changes. It
fetches only from the named local repository into a temporary repository and
removes that repository on exit. It does not alter the source checkout. Before
running it, commit intended changes on a task branch. Do not interpret success as
validation of uncommitted edits or of runtime compatibility.

Verification checks the exact upstream archive identity, patch hashes, IDs,
dependency order, and absence of unlisted patch files. Patches apply without
three-way fallback or conflict reuse. The resulting complete Git tree, including
bytes, deletions and modes, must equal the selected revision. All intentional
changes outside `fork/`, including workflows, documentation and module files,
must be represented. The reviewed `fork/` metadata is overlaid after replay;
payload patches cannot modify it. An upstream `fork/` path requires a deliberate
namespace migration.

The verifier proves reconstruction, not that patch contents or metadata are safe.
Run it without write credentials. Never execute a candidate's verifier with
publishing credentials; a future publisher must use its own trusted verifier.

## Update upstream

### Prepare an update locally

After fetching and independently verifying a new Google commit and its immutable
fork archive tag, run:

```sh
go run ./fork prepare -revision origin/trunk -upstream FULL_UPSTREAM_SHA -output /path/to/new-update-directory
```

Preparation requires a working checkout; Git metadata directories and bare
repository sources are rejected. A source subdirectory is normalized to its
checkout root. Fork metadata must contain regular files rather than symlinks or
submodules, and generated metadata files are replaced atomically.

The output directory's parent must exist. The directory must be new and outside
the source checkout and its Git metadata, including a shared worktree Git dir.
Only committed inputs are used; dirty source work is preserved. Git network
transports and lazy fetching are disabled. Fetch/archive creation remain explicit
maintainer steps before preparation.

The command first verifies the captured fork revision, seeds the old patch blobs,
and replays onto the selected forward upstream commit with three-way assistance
and conflict reuse disabled. On success, `repo/` contains a single-parent snapshot
whose parent is the captured fork revision. The refreshed queue preserves patch
IDs and rationale; generated payloads are full-index binary-capable Git diffs.
Disposable replay commits identify working checkpoints, not copyright ownership
or published feature history. Source notices and FORK_NOTICE remain authoritative.

`report.json` records captured inputs, status, candidate and verified tree;
`range.diff` compares the reconstructed old and new replay histories. A matching
upstream base returns `noop` without creating output. A conflict returns nonzero
and retains the patch ID, index and report for inspection. A patch made empty by
upstream stops for an equivalence/retirement decision. Reusing an existing output,
including an interrupted workspace, fails rather than overwriting evidence.

Preparation reruns exact reconstruction without conflict assistance and checks
that the selected local source ref has not advanced. It does not establish remote
freshness, runtime compatibility, consumer readiness, review approval or permission
to publish. Inspect conflicts manually, then refresh and reverify the resulting
queue; this command does not resume conflicted outputs automatically. Remove
disposable outputs after saving needed evidence and completing the update.

### Review and publish

Keep published `trunk` append-only through squash PRs. Original upstream commits
are preserved by immutable `upstream/<full-SHA>` tags, protected against updates
and deletion without bypass actors. Existing consumer pins stay reachable.
The manifest, rather than GitHub's ancestry counts, identifies the imported base.

1. Fetch `origin/trunk` and Google's `master`, then record their full SHAs. Verify
   the current published revision first. Work in disposable task worktrees;
   preserve dirty original worktrees.
2. Fetch Google's selected commit. Create its `upstream/<full-SHA>` tag, push it
   to this fork, and verify its remotely fetched peeled identity. A tag name alone
   is not evidence. Never move an existing archive tag. Keep the original LICENSE.
3. In a disposable checkout at the new upstream SHA, replay the manifest patches
   in order. Use `git apply --3way --index` for raw diffs emitted by the prepare
   command; `git am --3way` accepts mailbox-format patches only. Local rerere may
   assist manual adaptation. Inspect every resolution. In particular, preserve both upstream cancellation/back-edge
   checks and fork branch callbacks; never select an entire conflict side blindly.
4. Commit each adapted logical patch and export it using
   `git format-patch --full-index --binary`. Exclude `fork/`. Replace the queue,
   update its hashes and the upstream identity, and retain patch IDs and rationale.
   Retire redundant patches only with tests proving upstream now supplies them.
   `git range-diff` helps review adaptation; it does not replace final-diff review.
5. Put the reconstructed source plus metadata on a PR branch whose parent is the
   captured `origin/trunk`. For example, from that task branch, restore the replay
   tree's tracked paths with `git restore --source=<replay-SHA> --staged --worktree
   -- .`, then restore the reviewed `fork/` directory and stage its explicit paths.
   Commit and compare the result against the intended complete tree.
6. Run reconstruction, `internal/test.sh` on the supported Go/platform matrix,
   fork hook/parser regressions, and a clean external consumer with `GOWORK=off`.
   Test Sky's tester/coverage packages against this exact candidate; do not infer
   consumer success from a previous base. Missing checks block readiness.
7. Obtain independent review of the complete final PR diff and relevant runtime,
   test and failure paths. Record reviewed head SHA, findings, fixes, follow-up
   review, checks and limits in PR details. If trunk advances, recreate/review the
   update against its new head. Squash-merge only the exact reviewed candidate.
8. Verify the merged tree and PR state, then remove the task's remote branch,
   local branch and clean worktrees. Fetch/prune; preserve dirty or unmerged work.
   Keep upstream archive tags and tracking refs. Update consumer pins separately.

Prefer small feature patches and dedicated regression files to reduce conflicts.
Keep annotations parsing-only; compilation/evaluation of annotated inputs and
static checking remain unsupported. Unset hooks should preserve upstream behavior;
measure representative workloads before making performance claims.

## Recovery and rollback

Prove historical reconstruction using a fresh transport clone (`git clone
--no-local --no-tags` for a local source), with no object alternates and no access
to Google. Fetch the recorded archive explicitly from the fork, then verify.
Missing or mismatched archives must fail. Run `fork/recover.sh "$PWD"` from a
reviewed checkout to reconstruct its committed revision and historical `b2230ce`
using the fork's actual remote archives. The harness builds only the current
verifier, uses fresh transport clones without alternates, checks missing/moved
archive failures and removes temporary clones. It needs Go, Git and jq plus access
to the fork; it never fetches from Google. The read-only **Archive recovery**
workflow repeats this on PRs, trunk pushes, weekly and on manual dispatch. A
successful manual run does not establish that a scheduled event was delivered.

On failed replay or checks, leave published trunk and consumer pins alone. After
a faulty publication, repin consumers to the last good revision and submit a
corrective or revert PR restoring source, patches and manifest together. Never
force-reset trunk or move archive tags. Keep source inputs and review evidence
durably; expiring Actions artifacts are not the recovery mechanism.

## Automation boundary

The import process is manual. The **Upstream drift** workflow runs weekly and on
manual dispatch. It compares Google's observed `master` with `manifest.json`, adds
a comparison link to the run summary, and fails visibly when they differ. A failed
API request or invalid identity also fails; it cannot report freshness. The job
has read-only permissions and creates no branches or PRs. Detection is not publication.
Automatic conflict resolution and privileged publishing are outside this command.
Before adding publishing, bind trusted workflow/run/attempt/base/head/manifest/tree
and artifact identities, reject mismatches, and ensure candidate code never sees
write credentials. Keep one temporary update branch rather than a branch backlog.

## Measure hook costs

The isolated [benchmark harness](bench/README.md) pins a common upstream base,
fork runtime snapshot, exact toolchain and benchstat. It records ten alternating
samples for fixed parsing/evaluation workloads and separates nil/active hooks.
Run it explicitly on a quiet host; busy-machine results do not establish a stable
performance budget. Its development dependencies are excluded from the library
module by the nested-module boundary.
