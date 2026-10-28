# NX Witness information in saved sites

Starting with **v1.0.1**, importing a full NX Camera Info JSON report retains its
system, server, user, storage and licence sections alongside the cameras.

1. Export **all sections** as JSON in NX Camera Info. Reveal camera passwords
   there if you also want to transfer those known credentials.
2. In SADPTools, choose **Sites → Import site** and select that JSON file.
3. Review the camera and NX inventory counts, then import as a new site.
4. Select the imported site and click **NX Witness** in its toolbar.

The view includes **System**, **Servers**, **Users**, **Storage** and **Licences**.
Tables support searching, column sorting and pages of 50 rows. Expand a row for
additional metadata; licence keys and licence blocks have separate disclosure
controls. Empty/unknown values remain Unknown, while explicit zero and false
values are preserved. Integer fields are stored/displayed exactly, including
large permission bitmasks. Licence capacity is not inferred to be a remaining
activation count, and deactivation figures are shown as exported.

This is a read-only snapshot, labelled with its import time. Opening it does not
contact an NX server, check a live licence entitlement, log in as an NX user or
change any camera/server settings. Server IDs and storage relationships remain
source-system references.

The button appears **only if the site contains meaningful NX metadata**. Sites
created through camera discovery, manual sites and camera-only JSON imports
have no NX Witness area or button. Empty NX sections also leave it hidden.
An older import cannot recover discarded metadata: import a fresh full export
into a new site. Existing sites and their notes remain intact.

## Storage and transfers

The snapshot is an encrypted site attachment, protected by the same local vault
as saved credentials. Windows can use its existing automatic account protection;
passphrase vaults require unlocking. Only opening the view retrieves the snapshot.
Locking the vault closes the view and clears its displayed contents.

**Export site** includes NX metadata in the encrypted `.sadpsite` file. Importing
on another laptop re-encrypts it for that laptop's vault. Use v1.0.1 or newer to
view and restore these snapshots. Ordinary inventory/report exports and import
previews exclude NX user details and licence keys; the preview shows only counts.
Snapshots do not appear in history/log payloads. The source JSON remains a private
file and should stay out of Git.

The importer selects the NX export's documented fields and known inventory
properties. Unrelated fields such as server authentication keys, cloud tokens,
user password hashes and arbitrary plugin properties are excluded. Camera
credentials retain the separate [site-transfer workflow](site-transfer.md).
NX information is limited to 4 MiB per site and 10,000 rows per section; normal
site attachment limits also apply. Malformed known fields fail the import before
any partial site is saved.

## Validation

Automated tests cover encrypted persistence across restarts, vault locking,
transfer between separate vaults, preview/report privacy, malformed/duplicate or
foreign-site attachments, exact unknown/zero/false/large-integer values, and
conditional visibility for ordinary sites. Browser tests exercise all sections,
search, sorting, pagination, licence disclosure, unlocking and vault-lock cleanup
in light/dark desktop/mobile layouts, with accessibility checks. Screenshots use
synthetic data only.

The supplied updated export was tested locally on 28 October 2026: **31 cameras,
2 servers, 5 users, 6 storage locations and 10 licences** imported and survived an
encrypted export/import round trip. Validation used temporary encrypted stores
in RAM, removed afterwards. No camera or NX server was contacted, and private
user/server/licence values were not printed or committed.

Run `make check`, `make frontend`, and
`node frontend/tests/nx-witness-browser.mjs` (using the website's pinned Playwright
and axe dependencies). Native Windows/macOS interaction with your saved sites
still needs desktop testing beyond compilation and CI service tests.
