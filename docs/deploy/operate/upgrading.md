---
sidebar_position: 2
---

# Upgrading

There is one release (0.1.0), so there is no upgrade path to describe yet. This
page lists what to check between versions. It will grow with later releases.

## What is safe to know now

- **Outprint keeps no state.** There is no database and no cache of PDFs. An
  upgrade replaces containers. Nothing needs to be migrated.
- **The three services belong together.** The Gateway and the Worker run from
  one image, and the injector is built from the same tag. Upgrade all three to
  the same version.
- **Templates and Configurations are files.** They are mounted into the
  services, or built into the image. A new version can ship new shipped
  packages. Your own packages are not touched.

## Before you upgrade

1. Read the release notes (`CHANGELOG.md`) for the versions in between. Before
   version 1.0, a minor version can change settings or packages.
2. Read this documentation in the version that you install. The site has one
   version of the docs for every minor release.
3. Check your settings against that version. The service configuration is
   strict. An unknown key stops the Gateway at startup with a message, so a
   renamed setting cannot go unnoticed. The same holds for the `OUTPRINT_*`
   variables that are checked at startup.
4. Check your own Template and Configuration packages with the new version:

   ```sh
   docker run --rm -v "$PWD/templates:/opt/outprint/templates:ro" \
     -v "$PWD/configurations:/opt/outprint/configurations:ro" \
     outprint validate
   ```

   `validate` reports every problem of a package. Fix them before you roll out.
5. Check the [Compose file](../platforms/docker-compose.md) of the new version against yours.
   The Worker keeps its security settings in that file. The build of Outprint
   checks them, so do not weaken them while you merge changes.

## After you upgrade

1. Open a document in a Collection with a Binding and use **Export → Formatted PDF**.
2. If an author reports a message you do not know, look up its code in
   [When something goes wrong](../../write/when-something-goes-wrong.md).
   The codes are stable per version.

## Roll back

Start the previous image tags again. Nothing was migrated, so nothing needs to
be reverted.
