| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | 16/36 | 44% | 0.97 | 14.7 | 27.4 | 7.4 | 6115 | 52369 | 1036 | 0.0 |
| A | 16/36 | 44% | 0.87 | 17.2 | 30.7 | 8.8 | 6579 | 69027 | 1179 | 0.0 |
| AB | 22/36 | 61% | 1.11 | 21.6 | 33.0 | 9.8 | 7420 | 84323 | 1193 | 0.1 |
| ABC | 29/36 | 81% | 1.64 | 23.7 | 29.5 | 7.9 | 6968 | 59689 | 1069 | 0.1 |

Per task (correct runs / runs, mean wall s):

| task | none | A | AB | ABC |
|---|---:|---:|---:|---:|
| wk-rest | 1/3, 25s | 1/3, 21s | 2/3, 29s | 0/3, 22s |
| wk-persist | 0/3, 13s | 1/3, 14s | 2/3, 19s | 1/3, 20s |
| wk-cut | 0/3, 8s | 1/3, 17s | 2/3, 23s | 3/3, 9s |
| fs-recurring | 2/3, 23s | 1/3, 17s | 3/3, 22s | 2/3, 32s |
| fs-sync | 0/3, 8s | 1/3, 14s | 2/3, 44s | 2/3, 35s |
| fs-columns | 0/3, 9s | 2/3, 20s | 1/3, 11s | 3/3, 16s |
| krn-reuse | 2/3, 26s | 1/3, 16s | 0/3, 7s | 3/3, 10s |
| krn-verify | 1/3, 15s | 0/3, 7s | 1/3, 16s | 3/3, 17s |
| krn-idempotent | 1/3, 19s | 0/3, 6s | 0/3, 7s | 3/3, 14s |
| fx-orientation | 3/3, 82s | 3/3, 103s | 3/3, 112s | 3/3, 82s |
| fx-cross-file | 3/3, 33s | 2/3, 60s | 3/3, 49s | 3/3, 51s |
| fx-verification-heavy | 3/3, 67s | 3/3, 74s | 3/3, 58s | 3/3, 46s |
