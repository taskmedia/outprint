---
sidebar_position: 6
---

# Security notes for operators

What to expose, what to keep inside, and what to check after the install. The
reasoning and the tests are in [Security](../internals/security.md). The
Compose file already follows everything on this page.

## What to expose

- **Publish the injector, and nothing else.** The injector is the only service
  that browsers should reach. Do not publish the Gateway or the Worker.
- **Keep one address.** The Gateway sends no CORS headers and has no login of
  its own. The Outline session cookie is the credential, so it has to come from
  the Outline address. Route `/pdf/*` to the Gateway through the injector. The
  `ports` entry of the Gateway in the Compose file is for trying it out.
- **Terminate TLS in front.** The edge proxy ends TLS and sets the forwarding
  headers. A client that reaches the injector directly can forge
  `X-Forwarded-For`.

## What to keep inside

- **The Worker has no network.** It sits on an internal network that only the
  Gateway can reach. It has no route to the internet, to Outline or to the host.
  Keep it that way.
- **The Worker is locked down.** Non-root user, no extra privileges, all
  capabilities dropped, a read-only root file system, memory, process and disk
  limits. These are the settings of the shipped Compose file. The test suite
  fails if one of them is weakened, so merge your changes with care. At start
  the Worker logs `worker posture is weaker than the shipped baseline` if it
  finds a weaker setting.
- **Set a token between Gateway and Worker.** `OUTPRINT_WORKER_TOKEN` is
  optional. Set it when the network between the two is shared with other
  services. Do not reuse the value anywhere else.

## What the Gateway sends out

- **To Outline:** the user's cookie, so that Outline decides what the user may
  read. Nothing else gets the cookie.
- **To image hosts:** nothing with the cookie. Images come from Outline. A
  redirect is followed only to hosts you list in `imageHosts`, and only to public
  addresses. See [Image fetching](image-fetching.md).
- **To the Worker:** the document, its images and its settings.

The Gateway keeps no copy of a document or a PDF. Responses carry
`Cache-Control: no-store`.

## What the logs hold

The Gateway logs one line per request: job, document id, phase, kind, status and
duration. The Worker logs one line per failed job. Neither logs document text,
metadata values or cookies. Error messages shown to users carry no log excerpts
and no internal addresses.

## Check after the install

1. Reach the Gateway's port from outside. It must not answer.
2. Reach the Worker from outside, and from the Gateway's host network. It must
   not answer.
3. Open Outline through the injector. The **Download PDF** button appears in a
   Collection with a Binding.
4. Check the Content-Security-Policy of Outline if the button is missing. See
   [Injector and download button](injector.md#content-security-policy).
