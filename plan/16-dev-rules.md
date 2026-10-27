# 16. How a change is made: the ukubi dev rules

`15-architecture.md` says what the code looks like. This file says how a change gets there. It is written from the
standard's rollout (2026-10-27: mountain #49–55, ukubi-game #20–34, ukubi-kit #5–7, 15 doubt cycles, 18 reviews).
Each rule names the incident that taught it; "(×N)" means it happened N times.

It is for every agent working on our apps: a cloud or laptop session, or a fleet worker. ukubi-game's and
ukubi-kit's CLAUDE.md carry a short "Making a change" section that points here. The fleet's `change-loop` skill
carries the loop itself, because fleet workers can't read this repo (agent-fleet `packs/repo/skills/`, and
`fleet-shared/skills/` until ADR-0063's migration removes it). This file wins when they disagree.

## 1. The loop
Every non-trivial change goes through these steps in order. A step is skipped only on the owner's word.
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
   repo's CLAUDE.md.
8. **The merge,** after local verification and CI (§6 for a CI that is red for a reason outside the PR), then the
   branch re-synced (§6).

## 2. Planning
- **Check every fact against the code before writing it (×5).** Plans said tests swapped `liveEvery` (they only
  read it), that atlas spoke Connect's error body (it has no Connect dependency), three@0.186.0 (vendored: 0.186.1),
  "status check" in a client that checks no HTTP status (A1).
- **"Behaviour unchanged" and "tests unedited" are claims to prove, not defaults (×6).** List every test the change
  edits, with what each edit touches: setup, package clauses, imports and renamed identifiers are allowed in a
  refactor, an assertion never is. If the design can't keep a claim, change the design or drop the claim and label
  the change. *G1–G4: TestGate couldn't move without an import cycle; registering a route only when it exists turned
  a 501 into a 405. S1 listed 6 test edits; there were about 16.*
- **Order is behaviour (×5).** Registry order, map iteration, sort ties, log order within a tick and pass order are
  all observable somewhere. *three r186 sorts draws by creation id (S4); `LadderNames` fed the director's roll (S6);
  errors printed per civ, interleaved (kit step 7).*
- **Nil and empty differ on the wire.** A map or slice the old code always allocated stays allocated: `"apps":{}`
  and `"apps":null` are different frames (A1).
- **Error text is behaviour** when something compares it: a log that prints only on change, a client that matches a
  message, a test. Keep the strings, the wrapping and the decoder (A1).
- **Split and order the PRs.** A fix found during a refactor ships first, apart (`fix(sim)` #33 before #34). A
  security change ships early (G2). Fix the CI a PR needs before the PR (mountain #50, because the runner was down). A
  big change is split where each part can be proven alone (S4 into three, S5 into bundle then TypeScript).
- **Find the consumers outside the repo.** A log format, a metric name, a label or a wire field may be read by
  another repo, an alert or a dashboard. *Pushing logs over OTLP would have broken the Loki alerts that match on
  Alloy's `namespace` label.*
- **Owner decisions are not relitigated.** A plan that needs one changed asks the owner first.

## 3. Proving a refactor
- **The proof must reach the code being changed (×6).** Before trusting a golden, check that it exercises the
  changed path, and break the code on purpose to see it fail. *A frames check with no incident in it (S1); a golden
  on a fixture that never ran the passes (step 3); a gate that checked 3 of 5 looks (S4); a sim golden that never
  answered an incident (S6).*
- **Build the golden from the old code, and run it twice on main first.** A harness that differs from itself proves
  nothing. Find the randomness and pin or mask it:
  - map iteration (a fail-fast loop over a map: which query was asked first is random);
  - wall-clock fields (pass a fixed `now`, mask what reads `time.Now()`);
  - float sums over a map, which can move the last bits;
  - fail one thing per case, so which failure was counted is not random.
- **Drive the real path.** A golden through a fake skips the code being moved: go through the real client against a
  local server. Record everything the path writes: the return, the state, every broadcast, the counters.
- **Methods that worked:**
  - **goldens** of a transform's output, compared byte for byte (atlas's projection; the sim's frames, 38 runs, plus
    the control status body);
  - **mutation harnesses** for a build's refusals: each case breaks one input on a fresh copy and compares exit
    codes and error classes, not message text (kit, 36 cases);
  - **dist byte checks** under every engine and platform CI runs (Bun on Linux and macOS, Node);
  - **test names before and after** (`go test -v`), plus `git diff -M` of moved tests showing only package clauses,
    imports and renames (G4, S1–S3);
  - **old and new binaries side by side** over every flag and env combination: same exit codes and messages (G1).
- **Every new test gets a mutation.** Disable the code it guards (`|| false &&`), and check the test fails for that
  reason. A mutation that breaks the build (an unused import) is a false "caught".
- **A test needs a positive control.** A gate test that only checks refusals passes on a gate that refuses
  everything (G2).
- **Screenshots are not a check:** they aren't deterministic. Compare data (card dumps, a draw trace). And check that
  the trace isn't empty: the WebGL trace was `{}` until the prototype was patched.

## 4. Review
- **Independent, and given the code, not your conclusions.** The reviewer reads the diff and the code around it.
  Findings come back verified, with the path from a real input to the failure.
- **What reviews found after the tests were green:**
  - **docs, comments and commit messages claiming more than the code does (×11 of 18 reviews).** "1 in 400
    vertices" was 30 of 7730. Re-read every sentence you wrote against the code;
  - **error paths:** an early return that deleted dist and dropped the errors already found; a 401 from a service
    the game called that sent players into a sign-in loop (G3);
  - **unchecked input:** `[...refs]` on a string spread it into letters; a cell pattern accepted coordinates off
    the grid;
  - **guards with holes (×4):** the env ban, the key registry and the package-state check each missed a form (an
    alias, a method helper, a write through `slices.Sort`). Write AST guards on `go/types`, and give each a mutation;
  - **a gap you dismissed:** deleting a call passed every test (#52). "Unit tests cover it" is a claim to check;
  - **a fake of something we own** (S1), which the rules forbid;
  - **a build binary committed by mistake** (25 MB, #51). Check `git status` before each commit.

## 5. Engines and platforms
- **Sorts break ties by engine.** gltf-transform's weight sort puts the later slot first under V8 and the earlier
  under JavaScriptCore. Do the engine-dependent step yourself, in one order (ukubi-kit #6).
- **Math is the platform's.** Bun calls the host's libm, Node ships its own. Only quantization hides the
  difference, so CI checks dist under each engine on Linux and macOS.
- **Bundlers change code:** Bun turns `const`/`let` into `var` (TDZ errors disappear); tree-shaking shifts three.js
  object ids; `--chunk-naming '[name].[ext]'` overwrites the entry; hashed chunk names depend on the working
  directory; `bun build` doesn't clean its outdir; a stray `.js` beats its `.ts`; `bun test tools` is a substring
  filter (`./tools`); `bun add` writes `^` ranges (`-E`); TypeScript 6 defaults `types` to `[]`.

## 6. Git, PRs and CI
- **Re-sync after a squash merge** without a force push: fetch, then merge the old branch head into a branch made
  from main with `-s ours` (tree unchanged), and push. A force push is refused here (×2).
- **A merge's expected head is the full 40-character sha from `git rev-parse HEAD`,** never a short one expanded
  by guess (×2: 409s).
- **A tool call that timed out may have worked:** list the PRs before creating one again.
- **Strip the server's footer from a PR body** when the repo's rules forbid attribution (mountain: none at all).
- **A CI that is red for a reason outside the PR** (an unset secret, a runner down) is named in the PR, the PR is
  verified locally instead, and a job that can run without it is made to (ukubi-game's web job needs no secret).
- **A fresh GitHub runner is slow:** the first Chromium start took over 10 s (#51). Wait long, and report the exit
  code on failure.
- **Collisions git can't see:** two migrations with different file names and the same number (agent-fleet#318).
  Build the merge result, not the branch.
- **Processes:** wait on explicit PIDs, never a bare `wait`; never `pkill -f` (its pattern can match the session's own processes).
- **Live pins use frozen refs** (`refs/pull/N/head` at a sha), never a branch that moves.
- **Agents share the checkout:** a reviewer can mistake the main agent's edits for another session's, so say in its
  brief what is yours. Don't let an agent add repos to the session: they can't be removed.
