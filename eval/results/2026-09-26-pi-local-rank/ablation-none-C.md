| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | 15/36 | 42% | 1.00 | 16.3 | 25.0 | 6.3 | 6340 | 42003 | 856 | 0.0 |
| C | 29/36 | 81% | 1.50 | 27.6 | 32.3 | 7.1 | 7397 | 53909 | 981 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | none | C |
|---|---:|---:|
| wk-rest | 0/3, 21s | 2/3, 51s |
| wk-persist | 0/3, 14s | 2/3, 22s |
| wk-cut | 0/3, 13s | 3/3, 17s |
| fs-recurring | 1/3, 16s | 1/3, 24s |
| fs-sync | 2/3, 37s | 1/3, 16s |
| fs-columns | 2/3, 22s | 3/3, 21s |
| krn-reuse | 0/3, 8s | 3/3, 13s |
| krn-verify | 0/3, 7s | 3/3, 24s |
| krn-idempotent | 1/3, 11s | 3/3, 30s |
| fx-orientation | 3/3, 47s | 3/3, 54s |
| fx-cross-file | 3/3, 47s | 2/3, 72s |
| fx-verification-heavy | 3/3, 57s | 3/3, 43s |
