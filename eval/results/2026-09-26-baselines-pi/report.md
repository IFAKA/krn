| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | 14/36 | 39% | 0.94 | 17.7 | 24.7 | 6.4 | 5546 | 39342 | 881 | 0.0 |
| C | 32/36 | 89% | 1.50 | 28.1 | 35.7 | 8.6 | 8375 | 66670 | 1220 | 0.0 |
| T | 33/36 | 92% | 1.07 | 45.8 | 51.2 | 13.5 | 10838 | 120869 | 1643 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | none | C | T |
|---|---:|---:|---:|
| wk-rest | 0/3, 18s | 1/3, 25s | 1/3, 52s |
| wk-persist | 1/3, 15s | 2/3, 29s | 3/3, 42s |
| wk-cut | 1/3, 22s | 3/3, 15s | 2/3, 32s |
| fs-recurring | 1/3, 18s | 2/3, 36s | 3/3, 51s |
| fs-sync | 1/3, 19s | 3/3, 49s | 3/3, 47s |
| fs-columns | 1/3, 15s | 3/3, 23s | 3/3, 42s |
| krn-reuse | 0/3, 8s | 3/3, 11s | 3/3, 31s |
| krn-verify | 0/3, 9s | 3/3, 12s | 3/3, 60s |
| krn-idempotent | 0/3, 6s | 3/3, 21s | 3/3, 38s |
| fx-orientation | 3/3, 54s | 3/3, 87s | 3/3, 84s |
| fx-cross-file | 3/3, 48s | 3/3, 34s | 3/3, 42s |
| fx-verification-heavy | 3/3, 64s | 3/3, 85s | 3/3, 94s |
