# NexusAI mock evaluation

Measured 2026-11-25T07:19:35.390129+00:00 in Linux aarch64, Python 3.12.16, Docker Compose, mock provider. Dataset fictional-v1 includes 20 invoice PDFs and 30 policy questions (20 answerable and 10 unsupported).

Two clients run concurrently, one sequential invoice client and one sequential policy client. Each starts a fresh workspace every ten requests. Worker concurrency is two. End-to-end invoice latency includes polling at 50 ms. Percentiles use nearest rank.

| Measure | Observed |
|---|---:|
| invoice cases | 20 |
| policy questions | 30 |
| text invoices scored | 18 |
| unsupported document cases | 2 |
| nonmissing fields correct | 177 |
| nonmissing fields scored | 177 |
| field accuracy | 1.0 |
| missing fields correct | 3 |
| missing fields scored | 3 |
| invoice routing accuracy | 1.0 |
| validation precision | 1.0 |
| validation recall | 1.0 |
| answer rate on answerable | 1.0 |
| expected clause on answerable | 1.0 |
| citation quote validity | 1.0 |
| abstention recall | 1.0 |
| human reviewed answer support | Unavailable |
| invoice job failure rate | 0.0 |
| estimated cost aed | Unavailable |
| invoice end to end p50 ms | 395.34 |
| invoice end to end p95 ms | 446.65 |
| invoice enqueue http p50 ms | 3.28 |
| invoice enqueue http p95 ms | 8.05 |
| policy http p50 ms | 13.91 |
| policy http p95 ms | 28.17 |

Synthetic English labels share the fixture layout. Scan and Arabic cases are evaluated only for routing. Quote and expected-clause checks are automated. Human support ratings, real model quality, token usage and cost are unavailable.

Per-case outputs, citations, expected labels, per-rule confusion counts and dataset hash are in [report.json](report.json). Ratios use a 0–1 scale. A missing denominator is reported as unavailable.
