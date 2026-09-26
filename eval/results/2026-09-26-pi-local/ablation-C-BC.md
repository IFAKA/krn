| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| C | 31/36 | 86% | 1.61 | 28.7 | 32.0 | 7.7 | 8042 | 59974 | 1094 | 0.0 |
| BC | 30/36 | 83% | 1.41 | 27.7 | 35.5 | 7.6 | 7893 | 59202 | 1117 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | C | BC |
|---|---:|---:|
| wk-rest | 0/3, 30s | 0/3, 31s |
| wk-persist | 1/3, 25s | 3/3, 25s |
| wk-cut | 3/3, 14s | 3/3, 16s |
| fs-recurring | 3/3, 40s | 2/3, 33s |
| fs-sync | 3/3, 36s | 2/3, 42s |
| fs-columns | 3/3, 11s | 3/3, 46s |
| krn-reuse | 3/3, 15s | 3/3, 18s |
| krn-verify | 3/3, 15s | 3/3, 15s |
| krn-idempotent | 3/3, 17s | 3/3, 19s |
| fx-orientation | 3/3, 70s | 3/3, 69s |
| fx-cross-file | 3/3, 52s | 3/3, 32s |
| fx-verification-heavy | 3/3, 60s | 2/3, 81s |
