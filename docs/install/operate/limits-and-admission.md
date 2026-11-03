---
sidebar_position: 4
---

# Tune limits

**How-to.** Size the Gateway's admission control to your Worker replicas and your users.

Every limit on one render (size, pictures, time, pages) ends in a clear message,
and nothing is ever cut short. The numbers are on the [Limits reference](../../reference/limits.md);
the time and size limits are settings in the [Settings reference](../../reference/settings.md)
(`OUTPRINT_TIMEOUT`, `OUTPRINT_MAX_PAGES`, `OUTPRINT_MAX_PDF_BYTES`). This page
is about the other side: how many renders may run at once.

## What admission control does

A render is a heavy job, and one Worker runs one job at a time. The Gateway
decides who may start.

- **Slots** (`slots`): how many renders run at once. Set it to the number of
  Worker replicas. A request that finds all slots taken waits in a queue.
- **Queue** (`queueDepth`): how many requests may wait, in order of arrival.
  When the queue is full the request is refused at once; `0` means nothing
  waits. A request that waits longer than `maxWait`, or whose client leaves,
  drops out.
- **Per user** (`perUser`): how many renders one user may have, queued and
  running together. The user is the one **Outline confirms** for the session
  (`auth.info`, with the forwarded cookie); nothing else the client sends counts.

A document with an error never takes a slot. The status check behind the button
renders nothing and counts against nobody.

## When it says no

A refusal is `429 Too Many Requests` with a `Retry-After` header in whole seconds
and an error list. `code` is `busy` (no slot and no place in the queue, or the
wait ran out) or `user_limit` (the user already has a render); both have
`source: service`, so nobody has to fix the document. Authors see the message in
the [error catalogue](../../reference/error-messages.md#busy).

## Set it

In the service configuration, every key optional, defaults shown:

<!-- validate: service-fragment -->
```yaml
admission:
  slots: 1        # positive integer
  queueDepth: 4   # zero or a positive integer
  perUser: 1      # positive integer
  maxWait: 60s    # positive duration
```

The variables `OUTPRINT_ADMISSION_SLOTS`, `OUTPRINT_ADMISSION_QUEUE_DEPTH`,
`OUTPRINT_ADMISSION_PER_USER` and `OUTPRINT_ADMISSION_MAX_WAIT` override the file
and are checked the same way, so a typo stops the Gateway at startup instead of
keeping the default. They are in the [Settings reference](../../reference/settings.md#outprint_admission_slots).
The Compose file passes them through; on Helm the chart sets `slots` to the
number of Worker replicas (`worker.replicas`) and the rest is in
`serviceConfig.admission`.

To scale, add Worker replicas and raise `slots` by the same number. A larger
`queueDepth` trades longer waits for fewer refusals. The state lives in the
Gateway process: with several Gateway replicas each has its own limits, and the
Worker's own busy answer stays the backstop.

## No render cache

A PDF is made anew on every request and every response is `Cache-Control: no-store`,
so a PDF never outlives the user's permission to read the document.
