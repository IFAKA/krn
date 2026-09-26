| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | 35/36 | 97% | 3.56 | 15.3 | 16.4 | 5.8 | 7759 | 88070 | 1223 | 0.0 |
| policy | 36/36 | 100% | 3.36 | 16.4 | 17.8 | 6.0 | 9540 | 113602 | 1353 | 0.0 |
| map | 33/36 | 92% | 4.52 | 11.2 | 12.2 | 3.7 | 6406 | 49362 | 981 | 0.0 |
| tree | 33/36 | 92% | 3.92 | 12.3 | 14.0 | 5.2 | 7454 | 70515 | 1115 | 0.0 |

Per task (correct runs / runs, mean wall s):

| task | none | policy | map | tree |
|---|---:|---:|---:|---:|
| wk-rest | 2/3, 18s | 3/3, 21s | 0/3, 11s | 0/3, 18s |
| wk-persist | 3/3, 15s | 3/3, 15s | 3/3, 10s | 3/3, 11s |
| wk-cut | 3/3, 14s | 3/3, 12s | 3/3, 7s | 3/3, 12s |
| fs-recurring | 3/3, 21s | 3/3, 15s | 3/3, 13s | 3/3, 13s |
| fs-sync | 3/3, 13s | 3/3, 17s | 3/3, 12s | 3/3, 12s |
| fs-columns | 3/3, 15s | 3/3, 15s | 3/3, 10s | 3/3, 13s |
| krn-reuse | 3/3, 14s | 3/3, 16s | 3/3, 10s | 3/3, 11s |
| krn-verify | 3/3, 14s | 3/3, 17s | 3/3, 10s | 3/3, 12s |
| krn-idempotent | 3/3, 12s | 3/3, 14s | 3/3, 10s | 3/3, 10s |
| fx-orientation | 3/3, 19s | 3/3, 23s | 3/3, 16s | 3/3, 15s |
| fx-cross-file | 3/3, 21s | 3/3, 25s | 3/3, 17s | 3/3, 18s |
| fx-verification-heavy | 3/3, 22s | 3/3, 23s | 3/3, 17s | 3/3, 24s |

Cost at list price, as reported by Claude Code (total_cost_usd):

| variant | total USD | mean USD/run | krn calls/run |
|---|---:|---:|---:|
| none | 1.17 | 0.032 | 0.0 |
| policy | 1.34 | 0.037 | 2.0 |
| map | 0.81 | 0.023 | 0.0 |
| tree | 0.99 | 0.028 | 0.0 |

Total: 4.31 USD
