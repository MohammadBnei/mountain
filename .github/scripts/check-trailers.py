#!/usr/bin/env python3
"""Refuse AI provenance in mountain's commit messages and pull request text.

mountain's rule: a commit or a PR carries no AI trailer, session link or co-author. This reads every commit
message in BASE..HEAD, the PR title and the PR body, prints each offending line with where it is, and exits 1
if there is any.

    check-trailers.py BASE_SHA HEAD_SHA

The title and body come from the environment (PR_TITLE, PR_BODY), never from the command line or the script:
they are untrusted text. BASE_REF, if set and fetched (refs/remotes/origin/<BASE_REF>), also bounds the range:
commits already on the base branch are history, not this PR's.

A human commit with no trailers passes. Standard library only.

Warning: the rules apply to the text of this PR's own title and body too. Describing them with the very strings
they refuse (the domain, an example model id, a quoted trailer) fails the PR that does it; say "claude links"
and "model names" in words.
"""
import os
import re
import subprocess
import sys

# (name, pattern), all case-insensitive. The trailers and the footer are matched at the start of a line, where
# they are written, after any markdown a PR body wraps them in (quote, bullet, emphasis); a sentence that merely
# says "generated with" passes unless it names Claude. The link, the address and the model names match anywhere.
MD = r"^[\s>*+_`~-]*"
RULES = [
    ("Co-Authored-By trailer", re.compile(MD + r"co-authored-by\s*:", re.I)),
    ("Agent-Session trailer", re.compile(MD + r"agent-session\s*:", re.I)),
    ("Agent-Definition trailer", re.compile(MD + r"agent-definition\s*:", re.I)),
    ("Claude-Session trailer", re.compile(MD + r"claude-session\s*:", re.I)),
    ("Anthropic noreply address", re.compile(r"noreply@anthropic\.com", re.I)),
    ("'Generated with' footer", re.compile(r"^[\W_]*generated\s+(?:with|by)\b", re.I)),
    ("'generated with Claude'", re.compile(r"\bgenerated\s+(?:with|by)\s+claude\b", re.I)),
    ("claude.ai / claude.com link", re.compile(r"(?<![\w-])(?:[\w-]+\.)*claude\.(?:ai|com)\b", re.I)),
    # Model names: "claude-opus-...", "claude_opus_5", "Claude Sonnet", "Claude 3.5 Sonnet", "Opus 5",
    # "Sonnet 4.6", "claude-3-5-...". Not the word "claude" alone, not ukubi-claude-<host>, not CLAUDE_MODEL /
    # CLAUDE_CODE_OAUTH_TOKEN.
    ("model name", re.compile(
        r"\bclaude[ _-](?:opus|sonnet|haiku|fable)(?![a-z])"
        r"|\bclaude[ _-]?\d+(?:\.\d+)?[ _-]?(?:opus|sonnet|haiku|fable)(?![a-z])"
        r"|\b(?:opus|sonnet|haiku|fable)[ _-]?\d"
        r"|\bclaude-\d", re.I)),
]


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def scan(text):
    """(line number, line, rule name) for each offending line of text."""
    out = []
    for n, line in enumerate(text.splitlines(), 1):
        for name, pat in RULES:
            if pat.search(line):
                out.append((n, line.strip(), name))
    return out


def commits(base, head):
    cmd = ["rev-list", "--reverse", head, "--not", base]
    ref = os.environ.get("BASE_REF", "").strip()
    if ref:
        full = "refs/remotes/origin/" + ref
        if subprocess.run(["git", "rev-parse", "--verify", "-q", full], capture_output=True).returncode == 0:
            cmd.append(full)
    return git(*cmd).split()


def main(argv):
    if len(argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    base, head = argv[1], argv[2]
    bad = 0
    for sha in commits(base, head):
        for n, line, name in scan(git("show", "-s", "--format=%B", sha)):
            print(f"commit {sha[:12]} line {n}: {name}: {line}")
            bad += 1
    for label, var in (("PR title", "PR_TITLE"), ("PR body", "PR_BODY")):
        for n, line, name in scan(os.environ.get(var, "")):
            print(f"{label} line {n}: {name}: {line}")
            bad += 1
    if bad:
        print(f"{bad} offending line(s): mountain's commits and PRs carry no AI trailer, session link or co-author "
              "(amend or rewrite the message; edit the PR text).", file=sys.stderr)
        return 1
    print("no AI trailers, session links or model names in the PR's commits, title or body")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
