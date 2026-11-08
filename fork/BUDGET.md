# Logical sequence accounting contract

`Budget` accounts declared output units at selected interpreter operations. It
is optional; a nil budget preserves ordinary evaluation. It does not measure Go
heap, RSS, retained objects, capacity, CPU time, or all allocations. Revisions
must include both `resource-budget` and `sequence-growth-budget` to supply the
coverage below. Earlier pins retain their earlier coverage.

## Operation inventory

| Executed operation | Automatic charge before result allocation/mutation |
| --- | --- |
| String/bytes repetition by integer, either operand order | Complete output byte length |
| List/tuple repetition by integer, either operand order | Complete output element count |
| String/bytes `+` | Sum of operand byte lengths |
| List/tuple `+` | Sum of operand element counts |
| Immutable string/bytes/tuple `+=` | Same as the corresponding `+` |
| List literal (`MAKELIST`) | Literal slot count, after evaluating its elements |
| List comprehension (`APPEND`) | One element for each append, after evaluating that element and its filters |
| Empty list creation/result | Zero elements |
| Cooperating Go host work | Only units explicitly charged by that host |

The compiler folds adjacent same-kind literals before evaluation. Adjacent
string/bytes literals become constants: there is no runtime `+` charge. Adjacent
list literals become one `MAKELIST`: `[1, 2] + [3]` charges three slots. In
contrast, `a = [1, 2]; a + [3]` charges two slots, one slot, then three result
slots. Runtime chains of dynamic concatenations charge each intermediate result.
This is operation accounting, not a source-text cost model.

| Unmetered path | Limit and next acceptance proof before extending coverage |
| --- | --- |
| Mutable list `+= iterable`, `append`, `extend`, `insert` | Define append versus capacity units, exact-size fast paths, arbitrary/lying iterable behavior, self-extension, frozen/active-iterator errors, iterator cleanup and partial mutation |
| Tuple literals and compiler-generated tuples | Separate user output from closure/default/call machinery, preserve diagnostics and frozen semantics |
| Index/slice operations, conversions, joins, formatting and other ordinary builtins | Audit every allocation path; preserve validation order and bound intermediate outputs before allocation |
| Dict/set literals, comprehensions and mutation | Define duplicate/replacement units, hash/equality callbacks, collision work, failed insert behavior and resize bounds |
| JSON and other library encoders/decoders | Bound input, nesting, expansion, streaming and temporary buffers with realistic valid documents |
| BigInt arithmetic and numeric conversion | Bound bit length, intermediate arithmetic work and conversion output before allocation |
| Parsing, resolution and compilation, including folded constants | Apply source/token/AST/depth/bytecode limits; prove bounded failures in isolated workers |
| VM frames, arguments, iteration stacks, spare list capacity, Go object headers and evaluator bookkeeping | Separate actual allocator/worker memory controls; logical output counters are insufficient |
| Custom values, Go builtins, arbitrary host operations and threadless `Binary` | Hosts must bound capabilities, explicitly charge, propagate failures and cooperate with cancellation; arbitrary Go cannot be interrupted by this API |

These are unresolved coverage gaps, owned by runtime/host hardening. Documentation
does not resolve them. No claim of full heap isolation follows from success.

## Units and refusal order

Bytes mean encoded byte length, including for Unicode strings; elements mean
sequence output slots, not recursively retained object size. Concatenation
charges the entire result even if it reuses an empty operand's storage. Repetition
retains its existing intrinsic-size validation before charging. Concatenation
first establishes the matching built-in operand types, then sums their lengths
in `uint64` and charges before Go concatenation or backing-slice allocation. Two
valid nonnegative Go lengths fit that unsigned sum even if their signed sum
would overflow. An unlimited or absent logical budget still does not guarantee
that the output can fit a Go allocation.

Operand/element expressions run before their construction check. Their errors,
side effects, charges and allocations remain observable. A failed literal charge
happens after its operand stack/frame has already been allocated; that stack is
not included in the literal charge. Wrong operand types and frozen/actively
iterated mutable-list `+=` retain their normal errors without a concat charge.
Concatenating frozen source lists is allowed and returns a fresh mutable list.

A refused charge changes no counters and performs no guarded result allocation
or append. Successful earlier charges are cumulative and never refunded. A
comprehension can have a successfully charged local prefix before refusal; that
prefix is not returned, and the failed append leaves it unchanged. Prior element
expression side effects are not rolled back. Active iterators and call frames
must still be released. `BudgetError` remains the cause of a source-mapped
`EvalError`.

Zero limits are unlimited; overflow of a counter is always refused. Zero-length
operations remain legal at an exhausted counter. Keep each concurrently executing
thread's budget separate. Sharing immutable functions and frozen inputs across
threads is supported; sharing a mutable budget is outside its contract. Go race
checks validate the separate-budget case, not unsupported concurrent access.

## Verification and remaining limits

Run `go test ./starlark -run '^TestBudget'` and the race suite. Regression tests
cover exact/cumulative limits, Unicode/empty outputs, counter overflow, source
errors, frozen sources, iterator cleanup and independent threads. Large finite
allocation tests detect moving concat/literal charges after allocation; the
comprehension helper test detects writing to both growing and reused backing
slices before refusal. Mutation evidence is recorded with the corresponding
candidate assessment, rather than treating a passing positive test as proof of
placement.

An accepted source pin still needs complete final-diff independent review,
reconstruction and actual required CI. Raw full-package vet remains non-green
for the unchanged unpack optimization described in [VET.md](VET.md). The earlier
repetition behavior and default nil-budget evaluator remain subject to the full
semantic suite; this extension does not authorize suppressing existing failures.
