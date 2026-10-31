# Split the renderer into a Gateway and a network-less Worker

Rendering runs Pandoc and LuaLaTeX on content written by any wiki user. Research on Markdown→LaTeX fidelity showed that disabling raw TeX is not sufficient: math input can read local files (`\input`) and run Lua (`\directlua`), and Lua can read the environment and files regardless of restricted shell escape. Filters and reader flags reduce that surface but are not a boundary.

We run the service in two roles from one image. The **Gateway** talks to Outline (session cookie, collection binding, attachment download, metadata merge) and builds a job bundle. The **Worker** only runs Pandoc and LuaLaTeX on a bundle: no network egress, no secrets, no cookie, read-only template and configuration mounts, one job per instance, a fresh tmpfs job directory per job, and a restart after any timeout or kill. The default deployment is one Worker replica.

## Considered options

- **Single hardened container.** Simpler, but the user's session cookie sits in the same process tree, UID and `/proc` as the TeX child, and network egress cannot be denied per process. A Lua payload that gets past the filters could reach all of it.
- **In-container process sandbox (namespaces, bwrap).** Portable only where unprivileged user namespaces are allowed; default Kubernetes seccomp profiles often forbid them.
- **One Kubernetes Job or pod per render.** Strongest isolation, but needs cluster-operator permissions and adds latency. Not a fit for a self-hosted default.

## Consequences

- The Worker holds nothing worth stealing, so the security argument rests on the Worker being dumb, not on every filter being correct. Each layer (reader flags, filters, engine configuration, sandbox) is tested independently.
- Throughput scales by Worker replicas, not by parallel jobs in one instance. A single replica means one slow render delays others, so admission control (bounded queue, per-user limit) is part of the design.
- Compose and Helm deployments need two services and a network policy for the Worker. Security tests read the shipped security settings so that weakening them breaks the build.
