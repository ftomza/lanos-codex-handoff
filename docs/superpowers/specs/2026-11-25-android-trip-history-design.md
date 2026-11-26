# Android saved-trip history

The LIVE journal lists sessions grouped by their JSONL session IDs. Each
session opens a local read-only report with distance, estimated fuel,
average over jointly observed intervals, time coverage, and aligned
charts with connection and MIL markers. The original JSONL export remains
available. Existing Android journal v1 works without migration.

Read bounded private archive parts off the UI thread. Reject invalid
headers and mismatched rows. Missing parts, malformed rows, time gaps,
connection-generation changes, and fuel counter resets break applicable
series. Sum compatible counter deltas before reducing charts. Preserve
extrema and breaks when reducing points. Use elapsed time across parts.
Reports run offline and use the same Canvas renderer as the Go history.

## Empty recordings

The journal offers **Удалить пустые** for sessions whose retained parts
show only startup and adapter-search activity, without signal readings,
fuel increments, or diagnostic results. A stopped process can leave a
session without a footer. Its saved rows must still parse, and all
preceding parts must be present. Preserve sessions with unknown events,
dropped rows, or malformed parts. Manual cleanup with a count and
confirmation lets the owner choose when to remove local JSONL. Run a
fresh check under the journal writer's executor after confirmation.
