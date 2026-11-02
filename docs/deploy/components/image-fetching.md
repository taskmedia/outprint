---
sidebar_position: 2
---

# Image fetching

Images in a document are files in Outline. The Gateway fetches them before a
render. This page covers what an operator sets: which hosts a redirect may
reach, and what to do when Outline runs behind an internal address. Authors
find the rules for pictures in [Images and logos](../../write/images-and-logos.md).

**Fetching images is the Gateway's only outbound request besides Outline's
API.** The model:

- The user's cookie is sent to the Outline origin and nowhere else. The first
  request goes to the configured Outline URL, with the cookie.
- Redirects are followed by hand, at most 5, **without the cookie**, and only
  to hosts on the allow-list: the Outline origin and `imageHosts`. Only
  `http` and `https`, no credentials in the URL.
- Names are resolved by the Gateway, once per request. Every answer must be a
  public address: loopback, private, link-local (including the cloud metadata
  address), carrier-grade NAT, multicast, reserved and documentation ranges,
  and IPv4 written as IPv6, NAT64, 6to4 and Teredo forms are refused. The
  connection goes to the address that was checked, and the address is checked
  again when the socket connects, so a name that changes its answer cannot
  redirect the request (DNS rebinding).
- One exception: the configured Outline origin itself (host and port) may be
  at a private address, which the local development stack needs. No other
  host gets this.
- No proxy from the environment is used. A request is bounded by 30 s and a
  body by 10 MiB.

**`imageHosts`** in the service configuration lists the other hosts a redirect
may lead to, for example the file storage of an Outline that keeps files in
S3-compatible storage. It is empty by default, so such an Outline shows
`forbidden` with the host's name until the operator adds it:

<!-- validate: service-fragment -->
```yaml
imageHosts:
  - files.example.invalid        # ports 80 and 443
  - storage.example.invalid:9000 # this port only
```

Entries are lower-case host names, optionally with a port: no scheme, path or
wildcard. A redirect target needs a public address, so a storage host on a
private network cannot be listed.

## Outline behind an internal address

When the Gateway reaches Outline at an address such as `http://outline:3000`
while Outline's own `URL` setting is public (`https://wiki.example.invalid`, or
`http://localhost:8090` in the dev stack), Outline's redirect to a stored file
points at the public URL. Set `OUTLINE_PUBLIC_URL` on the Gateway to that
public URL: a redirect to exactly that origin is then fetched from
`OUTLINE_URL` instead, still without the cookie and under the same checks on
the address of the configured origin. It adds no host to the allow-list: a
redirect to any other host is refused as before. Public and internal URL must
use the same path prefix, if any. Not needed when `OUTLINE_URL` is the public
URL, or when files live in S3-compatible storage (use `imageHosts`).
