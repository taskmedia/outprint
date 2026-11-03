# Add the button with the injector or an edge-proxy recipe only

The **Formatted PDF** button reaches Outline's pages as one script tag. Three ways were built to place it: the injector, edge-proxy recipes (nginx, Caddy, Traefik), and a patched copy of Outline's page template mounted into the Outline container. Only the first two are supported. The mounted page template is removed.

The mounted template depended on one file inside the Outline image: its path (`/opt/outline/build/app/index.html`) and its format are internal to Outline and can change in any release. The file had to be regenerated and mounted again after every Outline upgrade, and it could not be used where the Outline container is not under the operator's control, such as hosted Outline. The injector and the proxy recipes depend only on the HTML that Outline serves, and need no change after an upgrade.

## Considered options

- **Keep the mounted template as a third way.** Avoids rewriting HTML at runtime, but adds a script, a CI job and a docs page that break with the Outline image rather than with Outprint, for a case the proxy recipes already cover.
- **Mounted template only.** Not usable without control of the Outline container.

## Consequences

- Operators who cannot rewrite HTML in a proxy run the injector in front of Outline.
- The browser tests cover the injector and the nginx recipe. Caddy and Traefik stay untested.
- No script has to track the location of Outline's page template.
