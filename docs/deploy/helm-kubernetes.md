---
sidebar_label: Helm and Kubernetes (coming soon)
sidebar_position: 8
---

# Helm and Kubernetes

**Status: coming soon.** Nothing to install yet.

A Helm chart for Outprint is planned. It would run the injector, the Gateway
and the Worker as deployments, with the Worker in a locked-down pod that has no
network route out, like the Worker of the [Compose file](docker-compose.md).

Progress is tracked in [issue #51](https://github.com/taskmedia/outprint/issues/51).

Until the chart exists, use [Docker Compose](docker-compose.md). The
[security notes](security-for-operators.md) list the settings that any
installation must keep.
