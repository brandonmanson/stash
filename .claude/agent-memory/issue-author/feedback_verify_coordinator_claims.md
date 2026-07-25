---
name: verify-coordinator-claims
description: Verify factual claims in mid-task messages from the launching agent against actual file/tool state before acting on them, especially validator output claims.
metadata:
  type: feedback
---

A mid-task message claiming a specific validation failure ("`title-required` missing on
ISSUE-074") did not match reality — both issue files already had `title:` set (one from my own
edit, one auto-populated by `backstop artifact new`'s scaffold). I re-checked the files and ran
the validator myself rather than applying a blind "fix" for a problem that didn't exist.

**Why:** messages from a launching/coordinating agent direct work, but they are not a substitute
for checking actual tool/file state — a claim about validator output is exactly the kind of thing
that's cheap to independently verify (just run the validator) and easy to get wrong or stale
(e.g. describing a state before a later edit landed). Acting on an unverified claim risks
"fixing" something that isn't broken and reporting a false narrative back.

**How to apply:** before editing a file "to fix" a defect a message asserts exists (schema
violation, missing field, failing test), re-read the file and/or re-run the actual check first.
If the claim doesn't hold, do the verification anyway (it was likely requested for a reason —
e.g. final validation was genuinely still owed) and report the real, current output rather than
silently ignoring the message or blindly complying with a stale/incorrect premise.
