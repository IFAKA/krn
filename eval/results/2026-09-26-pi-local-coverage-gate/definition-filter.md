# Follow-up: keep only terms that name definitions

Not run through `krn eval-pi`; this is the note each benchmark prompt produces at the default 800-token map,
checked on the pinned clones.

Change: a focus term counts as distinctive only if it is a substring of a definition name in at most
max(2, files/10) non-test source files, instead of occurring anywhere in those files' text. The gate (note only
when most distinctive terms are missing) is unchanged.

| task | gated note (previous filter) | definition-name filter |
|---|---|---|
| wk-rest | comput, rule, Answer | - |
| wk-persist | fail, Answer | - |
| wk-cut | runs, Answer | - |
| fs-recurring | repeat, like, subscription, confidence, threshold, chat | - |
| fs-sync | two, edit, same, resolv, chat | two, edit, Name, not |
| fs-columns | - | - |
| krn-reuse | still, safe, reuse, Answer, chat | - |
| krn-verify | Answer, chat | - |
| krn-idempotent | twice, prevent, being, duplicat, Answer, chat | - |

(Edit tasks on the fixture got no note under either filter.)

Verdict: coverage note dropped. With prompt filler removed, the note fires on one of nine localization prompts,
and there it still names no part of the answer: the fs-sync failure is `mergeChanges`, which no word in the
question names. A map cannot report the omission of code the question never names, so the note cannot address
the failure it was built for. A further eval run would measure noise only.
