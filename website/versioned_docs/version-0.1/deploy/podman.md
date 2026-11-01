---
sidebar_label: Podman (coming soon)
sidebar_position: 10
---

# Podman

**Status: coming soon.** A guide for running Outprint with Podman is planned.

The services are ordinary OCI images, so Podman should work. What is missing is
a tested guide, in particular for the Worker settings that the
[Compose file](docker-compose.md) applies: no network, a read-only root file
system, dropped capabilities and a memory limit. Until a tested guide exists,
use [Docker Compose](docker-compose.md).

Progress is tracked in [issue #51](https://github.com/taskmedia/outprint/issues/51).
