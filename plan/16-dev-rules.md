# 16. How a change is made: the ukubi dev rules

`15-architecture.md` says what the code looks like. This file says how a change gets there. It is written from the
standard's rollout (2026-10-27: mountain #49–55, ukubi-game #20–34, ukubi-kit #5–7), and each rule names the
incident that taught it. Unqualified PR numbers below are the repo the bullet names; the rollout's game PRs were
#21–#24 (config, the gate's upgrade check and session id, the error model, `internal/`), #25–#27 (the sim
`Control`, `Forge` and need-source seams), #28–#30 (town.js into registries), #31–#32 (Bun, then TypeScript) and
#33–#34 (the sim's fix, then its refactor).

It is for every agent working on our apps: a cloud or laptop session, or a fleet worker. ukubi-game's and
ukubi-kit's CLAUDE.md carry a short "Making a change" section that points here. The fleet's `change-loop` skill
carries the loop itself, because a fleet worker's context doesn't load this file (agent-fleet
`packs/repo/skills/`, and `fleet-shared/skills/` until ADR-0063's migration removes it). This file is canonical;
those are its condensed forms.

## 1. The loop
Every non-trivial change (any feature or refactor, or a fix that crosses a module, changes a wire format or claims
"behaviour unchanged") goes through these steps in order. A step is skipped only on the owner's word.
1. **A short plan.** What changes and what stays, each claim about today's code with its `file:line`, every test it
   edits and every behaviour it changes (§2).
2. **A doubt cycle.** A fresh agent is given the plan and the code (not your conclusions), read-only, and asked to
   disprove the plan. It reports numbered findings, each with a severity, its evidence and a correction. Every
   finding is taken or answered in the plan. A finding that touches an owner decision goes back to the owner.
   *The owner rejected a plan that skipped it ("Did you do the mandatory doubt cycle?").*
3. **The proof first, from the old code** (§3): the golden, harness or byte check is built and run on main before
   the change is written.
4. **The build,** in the PR shape the plan chose (§2, order and split).
5. **Local checks:** the repo's own suites, its gates, and the proof run again: identical, or only the plan's listed
   differences.
6. **An independent review** of the diff (§4). Every finding is fixed, or answered with a reason in the PR.
7. **The PR:** what changed, the proof and its result, every test edit and behaviour change named. Trailers per the
   repo's attribution rule (its CLAUDE.md; mountain's is `00-decisions.md`: none at all).
8. **The merge, on the owner's word.** A PR waits for it, unless the owner has told you to merge yourself; then it
   merges after local verification and CI (§6 for a CI that is red for a reason outside the PR), and the branch is
   re-synced (§6).

## 2. Planning
- **Check every fact against the code before writing it.** Plans have said tests swapped `liveEvery` (they only read
  it; mountain #51), that atlas spoke Connect's error body (it has no Connect dependency), three@0.186.0 (vendored:
  0.186.1; game #31), and "a status check" in a client that checks no HTTP status (plan/15 step 5's first doubt
  cycle).
- **"Behaviour unchanged" and "tests unedited" are claims to prove, not defaults.** List every test the change
  edits, with what each edit touches. A refactor may edit a test's setup, package clauses, imports and renamed
  identifiers, each listed in the PR; never an assertion (plan/15's "unedited" means this). If the design can't keep
  a claim, change the design, or drop the claim and label the change. *Game #21–#24: TestGate couldn't move without
  an import cycle, and registering a route only when it exists turned a 501 into a 405. Game #25's plan listed 6 test
  edits; there were about 16.*
- **Order is behaviour.** Registry order, map iteration, sort ties, log order within a tick and pass order are all
  observable somewhere. *three r186 sorts draws by creation id (game #28–#30); the ladder names' order fed the
  director's roll (game #34); errors printed per civ came out interleaved (kit #7).*
- **Nil and empty differ on the wire.** A map or slice the old code always allocated stays allocated: `"apps":{}`
  and `"apps":null` are different frames.
- **Error text is behaviour** when something compares it: a log that prints only on change, a client that matches a
  message, a test. Keep the strings, the wrapping and the decoder.
- **Split and order the PRs.** A fix found during a refactor ships first, apart (game #33 before #34). A security
  change ships early (game #22). Fix the CI a PR needs before the PR (mountain #50, when the runner was down). A big
  change is split where each part can be proven alone (town.js in three PRs; Bun, then TypeScript).
- **Find the consumers outside the repo.** A log format, a metric name, a label or a wire field may be read by
  another repo, an alert or a dashboard. *Pushing logs over OTLP would have broken the Loki alerts that match on
  Alloy's `namespace` label (plan/15's doubt cycle).*
- **A fix covers every path to the state it guards.** List them before writing it. *A4's first sign-out fix closed
  the open socket only; a socket down at the time, a page brought back with Back and a 403 at start reached the same
  reload into the sign-in, found by two more reviews (mountain #61).*
- **Owner decisions are not relitigated.** A plan that needs one changed asks the owner first.

## 3. Proving a refactor
- **The proof must reach the code being changed.** Before trusting a golden, check that it exercises the changed
  path, and break the code on purpose to see it fail. *A frames check with no incident in it (game #25); a golden on
  a fixture with no watch rows (mountain #52); a gate that checked 3 of 5 looks (game #28–#30); a sim golden that
  never answered an incident (game #34).*
- **Build the golden from the old code, and run it twice on main first.** A harness that differs from itself proves
  nothing. Find the randomness and pin or mask it:
  - map iteration (a fail-fast loop over a map: which query was asked first is random);
  - wall-clock fields (pass a fixed `now`, mask what reads `time.Now()`);
  - float sums over a map, which can move the last bits;
  - fail one thing per case, so which failure was counted is not random.
- **Drive the real path.** A golden through a fake skips the code being moved: go through the real client against a
  local server. Record everything the path writes: the return, the state, every broadcast, the counters.
- **Methods that worked** (each built for its change, not committed):
  - **goldens** of a transform's output, compared byte for byte (atlas's projection; the sim's frames over 38 worlds,
    seeds and intensities, plus the control status body);
  - **mutation harnesses** for a build's refusals: each case breaks one input on a fresh copy and compares exit
    codes and error classes, not message text (kit #7, 36 cases);
  - **dist byte checks** under every engine and platform CI runs (Bun on Linux and macOS, Node);
  - **test names before and after** (`go test -v`), plus `git diff -M` of moved tests showing only package clauses,
    imports and renames (game #24–#27);
  - **old and new binaries side by side** over every flag and env combination: same exit codes and messages
    (game #21).
- **Run a harness that starts servers alone.** Two at once collide on ephemeral ports and fail for no reason: the
  game's integration test failed beside atlas's binary matrix and passed alone (A4).
- **Every new test gets a mutation.** Disable the code it guards (`if false && bad` in Go, `if (false && bad)` in
  JS), and check that the test fails for that reason. A mutation that breaks the build (an unused import) is a false
  "caught".
- **A test needs a positive control.** A gate test that only checks refusals passes on a gate that refuses
  everything (game #22).
- **Screenshots are not a check:** they aren't deterministic. Compare data (card dumps, a draw trace). And check that
  the trace isn't empty: the WebGL trace was `{}` until the prototype was patched.

## 4. Review
- **Independent, and given the code, not your conclusions.** The reviewer reads the diff and the code around it.
  Findings come back verified, with the path from a real input to the failure.
- **What reviews found after the tests were green:**
  - **docs, comments and commit messages claiming more than the code does,** in most reviews. "1 in 400 vertices"
    was 30 of 7,730 (kit #6). Re-read every sentence you wrote against the code;
  - **error paths:** an early return that deleted dist and dropped the errors already found (kit #7); a 401 from a
    service the game called that sent players into a sign-in loop (game #23);
  - **unchecked input:** `[...refs]` on a string spread it into letters; a cell pattern accepted coordinates off
    the grid (kit #7);
  - **guards with holes:** the env ban, the key registry and the package-state check each missed a form (an alias,
    a method helper, a write through `slices.Sort`). Write AST guards on `go/types`, and give each form a mutation;
  - **a gap you dismissed:** deleting a call passed every test (mountain #52). "Unit tests cover it" is a claim to
    check;
  - **a fake of something we own** (game #25), which the rules forbid;
  - **a build binary committed by mistake** (25 MB, mountain #51). Check `git status` before each commit.

## 5. Engines and platforms
- **A comparator that never returns 0 leaves ties in an engine-defined order.** gltf-transform's weight sort puts
  the later slot first under V8 and the earlier under JavaScriptCore; a stable sort doesn't help. Do that step
  yourself, in one written-down order (ukubi-kit #6).
- **Math is the platform's.** Bun calls the host's libm, Node ships its own. Only quantization hides the
  difference, so CI checks dist under each engine on Linux and macOS.
- **Bun 1.4 (the bundler and the CLI):**
  - `const`/`let` become `var` (TDZ errors disappear);
  - tree-shaking shifts three.js object ids;
  - `--chunk-naming '[name].[ext]'` fails the build when a chunk and an entry share a name (keep `[hash]`);
  - hashed chunk names depend on the working directory;
  - `bun build` doesn't clean its outdir;
  - a stray `.js` beats its `.ts` when the import names `.js` (the TypeScript convention);
  - `bun test tools` is a substring filter (write `./tools`);
  - `bun add` writes `^` ranges (use `-E`).
- **TypeScript 6** defaults `types` to `[]`.

## 6. Git, PRs and CI
- **Re-sync after a squash merge** without a force push: fetch, make the branch again from main, merge the old
  branch head into it with `-s ours` (tree unchanged), and push it to the same branch name. That is a fast-forward,
  since the old head is a parent. A force push is refused here.
- **A merge's expected head is the full 40-character sha from `git rev-parse HEAD`,** never a short one expanded
  by guess (two 409s).
- **A tool call that timed out may have worked:** list the PRs before creating one again.
- **Strip the "Generated with Claude Code" footer** that a cloud session's instructions add to PR bodies: every
  ukubi repo forbids it, and mountain allows no attribution at all.
- **A CI that is red for a reason outside the PR** (an unset secret, a runner down) is named in the PR, the PR is
  verified locally instead, and a job that can run without it is made to (ukubi-game's web job needs no secret).
- **While the build runner is down, main builds only its newest commit** (`cancel-in-progress`): an image change
  that PR CI cannot build (a Dockerfile or build-push edit) waits until main has deployed, so it never stands
  between an earlier fix and its deploy (A6a, while #61's sign-out fix waited for the runner).
- **A fresh GitHub runner is slow:** Chrome's first start can take over 10 s (mountain #51). Wait long, and report
  the exit code on failure.
- **Collisions git can't see:** two migrations with different file names and the same number (agent-fleet#318).
  Build the merge result, not the branch.
- **Processes:** wait on explicit PIDs, never a bare `wait`; never `pkill -f` (its pattern can match the session's
  own processes).
- **Live pins use frozen refs** (`refs/pull/N/head` at a sha), never a branch that moves.
- **Agents share the checkout:** a reviewer can mistake the main agent's edits for another session's, so say in its
  brief what is yours. Don't let an agent add repos to the session: they can't be removed.
