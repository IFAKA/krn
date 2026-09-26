Experiment, not merged: `krn map` ended with a line naming focus words the map did not show (see README, Benchmarks, "Negative result"). Same manifest, model and machine as `2026-09-26-pi-local`; variant C only.

| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| C | 29/36 | 81% | 0.99 | 31.9 | 48.6 | 10.1 | 8808 | 84440 | 1381 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | C |
|---|---:|
| wk-rest | 0/3, 69s |
| wk-persist | 2/3, 32s |
| wk-cut | 3/3, 18s |
| fs-recurring | 3/3, 33s |
| fs-sync | 1/3, 23s |
| fs-columns | 3/3, 23s |
| krn-reuse | 3/3, 19s |
| krn-verify | 3/3, 49s |
| krn-idempotent | 3/3, 29s |
| fx-orientation | 3/3, 139s |
| fx-cross-file | 2/3, 62s |
| fx-verification-heavy | 3/3, 87s |

Verdict: not shipped. Compared with the previous C run (eval/results/2026-09-26-pi-local-rank):
correct is unchanged (29/36 both), but mean wall time rose from 32.3 s to 48.6 s and mean turns
from 7.1 to 10.1, so correct answers per minute fell from 1.50 to 0.99. The note fixed the case it
targeted (fs-recurring 1/3 -> 3/3); fs-sync stayed 1/3 because the model answered from the map and
ignored the note; wk-rest fell to 0/3 on cited line numbers (timers.js:18 instead of 20-22).
