"""Render the audited feature catalogue; all content stays readable without JS."""
from pathlib import Path
import html
import json

HERE = Path(__file__).resolve().parents[1]
esc = lambda value: html.escape(str(value), quote=True)


def render_features(base, icon):
    catalogue = json.loads((HERE / 'content/features.json').read_text())
    groups = catalogue['groups']
    features = [item for group in groups for item in group['features']]
    count = len(features)
    jumps = ''.join(f'<a href="#group-{esc(g["id"])}"><span>{i:02}</span>{esc(g["short"])}<small>{len(g["features"])}</small></a>' for i, g in enumerate(groups, 1))
    sections = []
    images = []
    for index, group in enumerate(groups, 1):
        cards = []
        for item in group['features']:
            name = item['id']
            # Build fails if a catalogue screen is missing, rather than shipping broken thumbnails.
            meta = json.loads((HERE / 'content/screenshots.json').read_text())[name]
            camera = item.get('capture') == 'real-camera'
            capture_kind = 'Real camera' if camera else 'Demonstration'
            light_version = meta['sha256']['light'][:12]
            dark_version = meta['sha256']['dark'][:12]
            for theme in ('light', 'dark'):
                for ext in ('png', 'webp'):
                    assert (HERE / f'public/assets/screens/{name}-{theme}.{ext}').is_file(), f'Missing {name}-{theme}.{ext}'
            points = ''.join(f'<li>{esc(point)}</li>' for point in item['points'])
            note = f'<p class="feature-boundary">{esc(item["note"])}</p>' if item.get('note') else ''
            cards.append(f'''<article id="feature-{name}" class="feature-card" data-feature="{name}" data-capture="{capture_kind}" data-light-version="{light_version}" data-dark-version="{dark_version}">
<a class="feature-screen" href="/assets/screens/{name}-light.png?v={light_version}" data-gallery="{name}" aria-label="Enlarge {esc(item['title'])} screenshot">
<img src="/assets/screens/{name}-light.webp?v={light_version}" data-screen="{name}" width="{meta['width']}" height="{meta['height']}" loading="lazy" decoding="async" alt="{esc(item['title'])} — SADPTools {capture_kind.lower()} screen">
<span class="enlarge-label">{icon('plus')} View screen</span></a>
<div class="feature-copy"><p class="feature-index">{index:02} / {esc(group['short'])}</p><h3>{esc(item['title'])}</h3><p class="feature-intro">{esc(item['text'])}</p><ul>{points}</ul>{note}
<p class="capture-caption" data-caption>{esc(item['caption'])}</p></div></article>''')
            images.append({'@type':'ImageObject', 'name': item['title'], 'description': item['caption'], 'contentUrl':base+f'/assets/screens/{name}-light.png?v={light_version}', 'thumbnailUrl':base+f'/assets/screens/{name}-light.webp?v={light_version}', 'caption':item['caption']})
        sections.append(f'''<section class="feature-group" id="group-{group['id']}" aria-labelledby="heading-{group['id']}"><div class="feature-group-heading"><span class="group-number">{index:02}</span><div><h2 id="heading-{group['id']}">{esc(group['title'])}</h2><p>{esc(group['intro'])}</p></div></div><div class="feature-card-grid">{''.join(cards)}</div></section>''')
    body = f'''<section class="features-hero"><div class="container"><a class="text-link" href="/">← SADPTools home</a><div class="features-hero-grid"><div><p class="eyebrow">THE COMPLETE TOOLKIT · WINDOWS + LINUX + APPLE MAC</p><h1>One toolkit.<br><span>The whole camera job.</span></h1><p class="section-intro">Find it. Set it up. See the picture. Solve the problem. Explore the tools that take a CCTV installation from the first discovery to a useful handover.</p><a class="button" href="/#downloads">Get SADPTools {icon('download')}</a></div><div class="feature-summary"><span class="summary-kicker">TAKE A CLOSER LOOK</span><strong>{count}<span>feature areas</span></strong><p>Actual desktop screens in light and dark mode. Open any image for the full view.</p><div class="summary-foot"><span>{count * 2} screenshots</span><span>No subscription</span></div></div></div>
<p class="demo-disclosure">{icon('camera')} <span><strong>Real app. Real camera previews.</strong> Screenshots were captured on Linux; the workspace is shared across all three platforms. The Live View, stream information, focus and PTZ images show an authorised camera feed, with its network address kept private. Other screens use synthetic devices and demonstration evidence. Captions explain what each image shows; one camera does not establish compatibility with every model.</span></p></div></section>
<section class="platform-update section" aria-labelledby="platform-heading"><div class="container"><p class="eyebrow">NEW: APPLE MAC SUPPORT</p><h2 id="platform-heading">Three platforms. One camera toolkit.</h2><div class="detail-grid"><article><h3>{icon('mac')} Native Apple Silicon + Intel</h3><p>A universal Mac app in a DMG or ZIP, targeting macOS 12 or later. Work with the same sites, discovery and camera tools as Windows and Linux.</p></article><article><h3>{icon('network')} Follow the network evidence</h3><p>Mac route, gateway and neighbour diagnostics join the existing connection tools. Keep Live View running in its own window while you investigate.</p></article><article><h3>{icon('download')} Updates for your platform</h3><p>Windows keeps its installer update flow. Mac checks select a universal package for manual installation; Linux uses its native or manual installer.</p></article></div><p class="catalogue-link">Mac builds are ready for testing; real Mac camera and desktop checks are still needed. <a href="/mac-help.html">Read the Mac installation guide →</a></p></div></section>
<div class="container features-layout"><aside class="feature-sidebar" aria-label="Feature guide"><p class="eyebrow">EXPLORE BY TASK</p><nav aria-label="Feature categories">{jumps}</nav><a class="cli-jump" href="#command-line">Also: the command line ↗</a></aside><div class="features-main"><div class="feature-search-bar"><div class="feature-search"><label for="feature-search">Find a feature</label><input id="feature-search" type="search" placeholder="Try PoE, recovery, recording…" autocomplete="off" aria-controls="feature-sections"></div><p id="feature-count" role="status">{count} feature areas · {len(groups)} categories</p></div><div id="feature-sections">{''.join(sections)}</div><div id="feature-empty" hidden><h2>No matching features.</h2><p>Try another word, or clear the search to explore the complete toolkit.</p><button type="button" class="button" id="clear-feature-search">Show all features</button></div></div></div>
<section id="command-line" class="section cli-section"><div class="container cli-grid"><div><p class="eyebrow">THE SAME ENGINE. YOUR TERMINAL.</p><h2>Prefer the command line?</h2><p>Alongside the Windows, Linux and Apple Mac desktop app, use the standalone <code>sadptools</code> CLI for discovery, interface enumeration, routed scans and troubleshooting. JSON output makes the results useful in scripts. The networking engine runs without a graphical desktop. Prebuilt CLI packages are available for Windows and Linux; the Mac downloads contain the desktop app.</p><a class="text-link" href="/#downloads">Choose a download {icon('arrow')}</a></div><div class="command-example"><p>EXAMPLE COMMANDS · NO DEVICE WRITES</p><pre><code>sadptools interfaces
sadptools discover --timeout 10s --json
sadptools --help</code></pre><span>Use the help output to review supported commands and options.</span></div></div></section>
<section class="feature-limits"><div class="container"><h2>Use the capability the device provides.</h2><p>SADP activation and network writes target compatible Hikvision equipment. ONVIF, ISAPI, SNMP, imaging, events and recording tools depend on the model, firmware and account permissions. Planning tools do not imply an automated device action. Current test releases need verification on your own authorised equipment.</p><p><a href="/windows-help.html">Windows installation and SmartScreen help →</a></p><p><a href="/mac-help.html">Apple Mac installation, Gatekeeper and updates →</a></p></div></section>
<section class="final-cta"><div class="container"><div><p class="eyebrow">YOUR NETWORK. YOUR TOOLS.</p><h2>Put the toolkit to work.</h2></div><a class="button" href="/#downloads">Download for Windows, Linux or Apple Mac {icon('download')}</a></div></section>
<dialog id="screen-viewer" aria-labelledby="viewer-title" aria-describedby="viewer-caption"><div class="viewer-top"><div><p id="viewer-counter" class="feature-index"></p><h2 id="viewer-title">Application screenshot</h2></div><button type="button" id="viewer-close" aria-label="Close screenshot viewer">Close ×</button></div><div class="viewer-canvas"><img id="viewer-image" src="/assets/screens/discovery-light.png" loading="lazy" alt="" width="1440" height="880"></div><div class="viewer-bottom"><p id="viewer-caption"></p><div class="viewer-controls"><button type="button" id="viewer-prev" aria-label="Previous screenshot">← Previous</button><div class="viewer-themes" role="group" aria-label="Screenshot colour theme"><button type="button" data-viewer-theme="light" aria-pressed="true">Light</button><button type="button" data-viewer-theme="dark" aria-pressed="false">Dark</button></div><button type="button" id="viewer-next" aria-label="Next screenshot">Next →</button><a id="viewer-original" href="/assets/app-light.png" target="_blank" rel="noopener">Full-size image ↗</a></div></div></dialog>'''
    schema = {'@context':'https://schema.org','@type':'CollectionPage','name':'SADPTools features and screenshots','url':base+'/features.html','description':'Explore camera discovery, configuration, Live View, diagnostics, recording tools and commissioning on Windows, Linux and Apple Mac.','about':{'@type':'SoftwareApplication','name':'SADPTools','operatingSystem':['Windows 10','Windows 11','Linux','macOS 12 or later'],'applicationCategory':'UtilitiesApplication'},'mainEntity':{'@type':'ItemList','numberOfItems':count,'itemListElement':[{'@type':'ListItem','position':i,'url':base+'/features.html#feature-'+f['id'],'name':f['title']} for i,f in enumerate(features,1)]},'image':images}
    return body, schema
