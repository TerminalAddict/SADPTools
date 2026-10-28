# Search visibility and submissions

Public website: **https://sadptools.com/**. Sitemap:
**https://sadptools.com/sitemap.xml**.

Search submissions help engines discover changes; they do not buy or guarantee
indexing, position one, or a particular result title. Google says recrawling can
take days to weeks. Search position also depends on the query, location,
competition and the usefulness/reputation of a page.

## What is implemented

- The homepage identifies SADPTools as free SADP discovery and CCTV installer
  software for Windows, Linux and Apple Mac.
- `/sadp-tool.html` explains Search Active Devices Protocol, Hikvision discovery,
  activation/network boundaries, and the distinction from other utilities.
  `sadptool` and `hiktools` appear in relevant, visible explanations rather than
  hidden keyword lists or misleading product aliases.
- `/cctv-camera-discovery.html` covers local SADP/ONVIF discovery, known camera IPs,
  routed scans, remote discovery and troubleshooting.
- `/cctv-installer-software.html` covers CCTV tools from commissioning to site
  handover, including encrypted transfers and imported NX Witness information.
- Pages have distinct titles/descriptions, canonical URLs, social metadata,
  accessible internal links and JSON-LD describing their actual content. The
  guides include article and breadcrumb data. There are no invented ratings or
  claims of Google endorsement/rich-result eligibility.
- `/index.html` redirects to `/`; existing HTTPS and bare-domain redirects remain.
- The sitemap lists fourteen public HTML pages, including eight guides and their index. `lastmod` is retained when content
  has not changed; `content/page-updates.json` records content hashes and dates.
  Rebuilding or re-pinging alone does not refresh the dates.
- `robots.txt` advertises the sitemap and excludes private/report/download paths.
  Access controls still protect private data; robots directives are not security.
- `public/indexnow-key.txt` is a **public ownership-verification token**, not a
  password or search-account credential. It is intentionally deployed and tracked.
  Private SSH/report credentials and submission logs remain excluded from Git.

## Normal updates from Sparx

```sh
cd /home/paul/git-repos/SADPTools
npm --prefix web run build
python3 web/scripts/deploy.py
```

After a successful upload, deployment automatically calls IndexNow for new or
changed sitemap pages. It compares their published HTML with the local build and
checks the public key before making a single batch request. It never submits
private reports, downloads, credential files or arbitrary external URLs.
Unchanged pages are skipped. The command does not notify Google.

If deployment succeeds but submission fails, the error explicitly distinguishes
the two. Fix the reported connectivity/key/rate-limit problem and run:

```sh
python3 web/scripts/submit_search.py
```

Preview without network requests:

```sh
python3 web/scripts/submit_search.py --dry-run
```

Deliberately re-submit all fourteen pages when needed:

```sh
python3 web/scripts/submit_search.py --all
```

Do not run a frequent ping cron job. Repeated submissions do not force higher
rankings or faster indexing. `--all` is for a deliberate refresh, not each visit
or download. Deployment can skip notifications with `--skip-search-submit`.
Submission receipts live in ignored `web/.cache/search-submissions.jsonl`; the
last accepted page hashes are in `web/.cache/indexnow-submitted.json`.

HTTP **200** means receipt; HTTP **202** means receipt with key validation pending.
Neither response confirms crawling or indexing. IndexNow shares requests across
its participating engines, currently listed as Bing, Yandex, Naver, Seznam,
Yep and Amazon. A single submission is sufficient; we do not send duplicate
batches to every participant. See the [IndexNow documentation](https://www.indexnow.org/documentation)
and [participant list/FAQ](https://www.indexnow.org/faq).

## Google: one-time owner setup

The owner has confirmed Google Search Console verification and submission of the
sitemap. Google can revisit that same sitemap as new guides are added; normal
updates do not require another verification. The steps below remain a reference
for setting up another property or restoring verification. This checkout has no
Google account credentials and does not make authenticated Google API calls.

1. Sign in at [Google Search Console](https://search.google.com/search-console/).
2. Add a **URL prefix** property for exactly `https://sadptools.com/`.
3. Choose **HTML tag** verification. Copy the complete
   `<meta name="google-site-verification" ...>` tag Google supplies and provide it
   for installation in the page builder. This is a public verification token;
   do not send your Google password, cookies or access tokens. Once deployed,
   click **Verify**. Alternatively, use the Domain property and add the supplied
   DNS TXT record through your DNS provider.
4. Open **Sitemaps**, enter `sitemap.xml`, and click **Submit**.
5. Use **URL inspection** for the homepage and the important guide URLs.
   Run the live test and select **Request indexing** where available.
6. Retain the verification tag/file or DNS record. Do not remove it after success.

Google has [retired its public sitemap ping endpoint](https://developers.google.com/search/blog/2023/06/sitemaps-lastmod-ping).
The general Indexing API is not a replacement for ordinary marketing pages.
For routine updates, keep the sitemap current; use URL inspection for a few
important revised pages. See [Google's recrawl instructions](https://developers.google.com/search/docs/crawling-indexing/ask-google-to-recrawl).

Review **Page indexing** for excluded pages and **Performance → Search results**
for impressions, clicks and average position. Filter for `sadp`, `sadptool`,
`sadptools`, `Search Active Devices Protocol`, `cctv installer software`,
`cctv camera discovery`, `hikvision sadp`, `hiktools` and `cctv tools`. Compare
several weeks of data rather than one personalised search result. Relevant links
from the author's website, installer documentation and genuine product coverage
can help people discover the site; avoid bought links or mass-submission services.

## Other search engines

| Engine | Submission route and limits |
| --- | --- |
| Bing | IndexNow is implemented. For indexing reports, sign in to [Bing Webmaster Tools](https://www.bing.com/webmasters/), add/import the site and submit the sitemap. [Bing recommends IndexNow](https://www.bing.com/webmasters/help/URL-Submission-62f2860b). |
| Yandex, Naver, Seznam, Yep, Amazon | Receive the shared IndexNow notification; no claim that each has indexed the site. Optional webmaster accounts provide engine-specific reports. |
| Yahoo | [Yahoo directs website submissions to Bing](https://help.yahoo.com/kb/SLN2217.html). This is not an independent Yahoo indexing confirmation. |
| DuckDuckGo | Many traditional links come from Bing; [DuckDuckGo explains its sources](https://duckduckgo.com/duckduckgo-help-pages/results/sources). There is no separate submission receipt from DuckDuckGo here. |
| Brave | Its [URL refresh form](https://search.brave.com/submit-url) can request a refetch of an updated page. Use the homepage and important revised guides if needed. A form request does not guarantee inclusion. |
| Baidu | [Baidu Search Resource Platform](https://ziyuan.baidu.com/linksubmit/url/) provides submission tools. Account/site verification may be required. No Baidu account access is configured here, so no authenticated submission is claimed. |

## Deployment receipt — 28 October 2026

- All eight public pages deployed and checked against the local HTML.
- IndexNow returned **HTTP 202**, accepted with key validation pending, for the
  complete eight-page batch. This is a submission receipt, not an indexing report.
- Brave's official refresh form displayed **Success / Thank you for your
  submission** for each of the eight URLs. Receipts are in ignored
  `web/.cache/brave-submissions.json`.
- At the initial deployment, Google submission was waiting for the owner. The owner subsequently confirmed verification and sitemap submission.
- An attempted homepage submission through Baidu's public form redirected to
  its account login page; no Baidu submission was confirmed.
- Live validation passed for eight HTML pages and two stylesheets. Website tests
  passed (213 existing PHP/HTTP assertions plus seven search-workflow tests).
  Live browser checks passed 32 route/theme/layout combinations with no automated
  accessibility violations. The homepage alias returned 301; missing pages
  returned 404; private records/configuration returned 403 and the report required
  authentication (401). All seven desktop download targets remained available.

## Checks

```sh
npm --prefix web test
node web/scripts/check-browser.mjs # with the local preview running
bash web/scripts/validate.sh https://sadptools.com/
```

Synthetic tests cover verification and deployment mismatches, failed batches,
accepted/pending responses, changed-page filtering, dry runs, private/external URL
exclusion and stable sitemap dates. Browser checks cover the new guides in both
themes and desktop/mobile sizes. Public deployment checks should also verify
redirects, the key, sitemap, robots rules and the private-path access controls.

## ChatGPT search availability

OpenAI's [crawler documentation](https://developers.openai.com/api/docs/bots)
identifies **OAI-SearchBot** as the search crawler. The public site allows it and
has an explicit group with the same private-path exclusions as other crawlers.
The existing GPTBot policy is unchanged; search access and training permissions
are independent. There is no login or client-side rendering requirement for the
guides, and the same HTML is served to readers and search crawlers.

The guide index at **https://sadptools.com/guides.html** links all eight articles.
Five task walkthroughs were added for activation, IP/DHCP changes, routed cameras,
Linux/Mac setup and complete site transfers. Instructions were checked against
the current UI and v1.0.1 documentation. Screenshots identify demonstration data
and platform captures; no device writes were performed to produce these guides.
Each article has a direct answer, prerequisites, steps, verification/limitations,
section anchors, related links, an author, review date and article/breadcrumb
structured data. Review the stated version/date when workflows change.

Check deployment using:

```sh
python3 web/scripts/check_search_access.py
```

This verifies public HTML, canonical URLs, headings, robot rules and script-free
text with an OAI-SearchBot user-agent string. The requests originate from Sparx,
**not OpenAI's published crawler IP ranges**. They establish website accessibility
from this host, not proof that OpenAI fetched, indexed, cited or recommended a
page. Results are saved in ignored `.cache/search-access.json`. If a host firewall
or CDN is added later, review its crawler restrictions against OpenAI's published
IP ranges; do not disable general protections or trust user-agent strings as
identity proof.

IndexNow notifications reach participating search engines; they are not a direct
ChatGPT submission. No general website-registration endpoint was established in
the official OpenAI documentation checked for this work. No model-training or
recommendation guarantee is claimed. Useful public documentation makes the
information available when a search/retrieval system chooses to use it. It does
not install knowledge in every conversation. No hidden recommendation instructions,
tracking service, paid submission or site-specific ChatGPT account is added.

### Guide expansion deployment — 28 October 2026

- Published the eight-guide index and five new task walkthroughs; the sitemap now
  contains fourteen public pages. IndexNow returned **HTTP 200** for the changed
  fourteen-page batch. Future unchanged builds do not repeat submissions.
- All fourteen public pages returned the expected HTML with the search-crawler
  user agent. The text is available without JavaScript; canonical URLs and robot
  rules passed the access audit.
- The session's hosted web-search tool returned no results for a site-restricted
  search and could not open the site. Its reason was not established. This is
  recorded separately from the successful direct HTTP/browser checks and is not
  evidence that OpenAI has indexed the content.
- Live validation passed fourteen HTML pages and two stylesheets. All 56 browser
  route/theme/layout checks passed, along with the 213 existing website assertions
  and nine search/content tests. Private paths remained forbidden/authenticated,
  and the seven desktop package targets remained present.
- Google verification and the original sitemap submission were confirmed by the
  owner before this work. The sitemap URL is unchanged. Its next crawl can
  discover the added guides; Search Console URL inspection remains an optional
  way for the owner to request indexing of a few important new pages.
