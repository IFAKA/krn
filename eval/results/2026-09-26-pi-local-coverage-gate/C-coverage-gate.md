Experiment, not merged: `krn map` ended with a line naming focus words the map did not show (see README, Benchmarks, "Negative result"). Same manifest, model and machine as `2026-09-26-pi-local`; variant C only.

| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| C | 34/36 | 94% | 1.63 | 29.9 | 34.7 | 9.1 | 8485 | 75627 | 1161 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | C |
|---|---:|
| wk-rest | 2/3, 36s |
| wk-persist | 3/3, 28s |
| wk-cut | 3/3, 14s |
| fs-recurring | 3/3, 48s |
| fs-sync | 2/3, 28s |
| fs-columns | 3/3, 9s |
| krn-reuse | 3/3, 13s |
| krn-verify | 3/3, 14s |
| krn-idempotent | 3/3, 16s |
| fx-orientation | 3/3, 73s |
| fx-cross-file | 3/3, 58s |
| fx-verification-heavy | 3/3, 80s |

Verdict: not shipped; inconclusive. Aggregate beats the previous C run (eval/results/2026-09-26-pi-local-rank:
29/36, 1.50 correct/min) at 34/36, 1.63 correct/min, but the gain is not attributable to the gate. The gate
removed the note on only 3 tasks (fs-columns, fx-cross-file, fx-verification-heavy); 8 of the other 9 got a
byte-identical note to the ungated run (eval/results/2026-09-26-pi-local-coverage) and still swung widely
(wk-rest 0/3 -> 2/3, fs-sync 1/3 -> 2/3, krn-verify 49 s -> 14 s mean). Run-to-run noise at 3 seeds is about
as large as the measured difference. The notes also still list prompt filler ("Answer", "chat", "two",
"same"), so the distinctive-term filter needs work before another measurement.
