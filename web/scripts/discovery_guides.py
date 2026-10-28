"""Original, task-focused guides for installers; no search-only or hidden copy."""
from html import escape
import re
from task_guides import TASK_GUIDES


GUIDES = [
    {
        "file": "sadp-tool.html",
        "title": "SADP Tool for Hikvision Cameras | SADPTools",
        "description": "Understand Search Active Devices Protocol and use SADPTools to discover compatible Hikvision cameras on Windows, Linux and Mac. Free desktop downloads.",
        "heading": "SADP discovery for Hikvision cameras",
        "intro": "SADPTools is a free, independent desktop application for finding and commissioning compatible Hikvision cameras and recorders on Windows, Linux and Apple Mac.",
        "image": "discovery",
        "caption": "SADPTools discovery inventory. Linux screenshot with clearly labelled demonstration devices.",
        "body": '''
<h2>What is SADP?</h2>
<p><strong>SADP means Search Active Devices Protocol.</strong> Hikvision equipment uses it to announce device information on the local network. A SADP tool can show a camera's address, model, serial number, firmware and activation status when the device supplies those values.</p>
<p>This is useful when a camera is new, its IP address is unknown, or its existing address does not fit your laptop's subnet. SADP discovery can find compatible devices with an out-of-subnet address when the laptop and device still share the same local Layer 2 network. Discovery does not by itself make the camera's web page reachable.</p>
<h2>A SADP tool for Windows, Linux and Mac</h2>
<p>SADPTools brings discovery together with supported activation, network configuration, Live View, saved sites and diagnostics. Windows users can choose an installer or standalone ZIP. Linux users have DEB, RPM and a manual installer. Mac users have a universal app for Apple Silicon and Intel.</p>
<p>If you searched for <strong>sadptool</strong> or a <strong>Hikvision SADP</strong> alternative, start with the same discovery workflow below. SADPTools is developed by Paul Willard and is not an official Hikvision product or affiliated with Hikvision.</p>
<h2>Find a Hikvision camera in four steps</h2>
<ol><li><a href="/#downloads">Download SADPTools for your operating system</a> and connect the laptop to the camera's network.</li><li>Choose the connected network adapter, or scan all suitable adapters when you are unsure which one carries the cameras.</li><li>Start discovery. Use the progress indicator and diagnostic log to follow the scan; you can cancel it or refresh again without losing the inventory.</li><li>Confirm the model, MAC address and serial number before selecting a device for configuration. Save the equipment to a named site when you want to keep it.</li></ol>
<h2>Discovery, activation and network changes are different operations</h2>
<p>Discovery reads the information a device advertises. Activation sets the first administrator password on compatible inactive equipment. IP configuration changes a supported device's network settings and needs the appropriate credentials and local adapter. These actions are explicit; a scan never automatically activates or readdresses a camera.</p>
<p>Password recovery uses a method supported by that device, such as previously configured security questions or a support-provided recovery code. It does not bypass security. Model, firmware and account permissions determine which actions are available.</p>
<h2>Why a camera may not appear</h2>
<p>SADP uses UDP port 37020 and multicast address <code>239.255.255.250</code>. VLAN boundaries, isolated Wi-Fi clients, host firewalls, another application holding the discovery port, or the wrong adapter can prevent replies. A camera behind a router may be reachable in a browser while remaining invisible to local multicast discovery.</p>
<p>Use the <a href="/cctv-camera-discovery.html">CCTV camera discovery guide</a> to choose between local discovery, adding a known IP, scanning a routed range and using a remote probe.</p>
<h2>Is SADPTools the same as HikTools or HiTools?</h2>
<p>No. Searches for <strong>hiktools</strong> can refer to other Hikvision-related utilities. Hikvision also uses the HiTools name for its own tools. SADPTools is a separate application with its own interface and supported workflows. Check the application name and the <a href="/features.html">feature catalogue</a> when choosing a download.</p>
<h2>Keep the evidence with the installation</h2>
<p>Save discovered cameras, add notes and locations, retain known credentials in protected storage, and prepare a commissioning report. See the <a href="/cctv-installer-software.html">CCTV installer workflow</a> for moving from discovery to a portable site handover.</p>''',
    },
    {
        "file": "cctv-camera-discovery.html",
        "title": "CCTV Camera Discovery: Local & Routed Networks | SADPTools",
        "description": "Find CCTV camera IP addresses with SADP, ONVIF, direct IP checks or a routed network scan. Practical discovery and troubleshooting on Windows, Linux and Mac.",
        "heading": "CCTV camera discovery, on site or across a route",
        "intro": "Choose the discovery method that matches the network. SADPTools combines local camera discovery, known-IP access and explicit IPv4 scans in one Windows, Linux and Mac application.",
        "image": "add-ip",
        "caption": "Add-by-IP setup in SADPTools. Demonstration screenshot taken before a network request.",
        "body": '''
<h2>Start with where the camera is connected</h2>
<div class="guide-table-wrap" role="region" aria-label="Discovery methods comparison" tabindex="0"><table><caption>Choosing a camera discovery method</caption><thead><tr><th scope="col">Your situation</th><th scope="col">Start here</th><th scope="col">What it needs</th></tr></thead><tbody>
<tr><td>Hikvision camera on the same local network; IP unknown</td><td>SADP discovery</td><td>A suitable local adapter and multicast replies</td></tr>
<tr><td>Compatible camera from another manufacturer</td><td>ONVIF discovery</td><td>ONVIF discovery enabled and reachable on that network</td></tr>
<tr><td>You know the camera's IP, including on a routed network</td><td>Add a camera by IP</td><td>A working route and reachable camera services</td></tr>
<tr><td>You know the authorised subnet or address range</td><td>IPv4 network scanner</td><td>An explicit range, reachable services and bounded probes</td></tr>
<tr><td>You need local discovery on a remote network</td><td>Trusted SSH discovery host</td><td>A host on that network running the compatible sadptools CLI</td></tr>
</tbody></table></div>
<h2>Find a camera when you do not know its IP address</h2>
<p>Connect to the same camera VLAN or physical network and run local discovery. Select the relevant Ethernet or Wi-Fi adapter, or use all suitable adapters. SADPTools collects the information devices actually return, including IP address, model, MAC address, serial number and firmware where available. Unknown fields stay unknown.</p>
<p><a href="/sadp-tool.html">Search Active Devices Protocol (SADP)</a> can identify compatible Hikvision equipment even when its configured IP is outside your laptop's subnet, provided local Layer 2 discovery can reach it. ONVIF discovery provides another route for equipment that supports and enables that protocol. Neither method promises to detect every powered device.</p>
<h2>Add a camera on a different subnet</h2>
<p>A working route is enough for many direct-IP operations; the camera does not have to share your laptop's subnet. Use <strong>Add a camera by IP</strong> and enter its address. SADPTools checks reachable services and detects an available HTTP or HTTPS web interface. You can then save the identified device to a site.</p>
<p>Routed web access does not transport SADP multicast. It also does not guarantee that activation or SADP network changes will work remotely. Treat service reachability and local discovery as separate checks.</p>
<h2>Scan a known CCTV network</h2>
<p>Use the IPv4 scanner for a specific network you are authorised to inspect. Calculate a range from a CIDR prefix or enter start and end addresses, then choose service probes, timeout and concurrency. Keep the scope narrow enough to investigate the results. Cancellation retains observations already collected.</p>
<p>A silent address is not proof that it is unused: ICMP or service probes may be blocked, a device may be asleep, or a camera may use an unexpected port. Confirm address availability before changing a camera's IP.</p>
<h2>When local discovery must happen remotely</h2>
<p>A trusted SSH host on the remote camera network can run the shared sadptools CLI and return discovery observations. Verify its host-key fingerprint and install the compatible CLI there first. The desktop retains the remote origin of the results; this does not turn remote observations into permission for local Layer 2 writes.</p>
<h2>A short troubleshooting sequence</h2>
<ol><li>Check power or PoE and the switch link, then confirm the laptop and camera VLANs.</li><li>Review the selected adapter and the discovery log for socket, multicast or port-conflict errors.</li><li>Check the firewall rules for the requested discovery or camera service; retain the protections your network requires.</li><li>If the IP is known, try direct-IP access and inspect the detected HTTP/HTTPS interface. Browser access can succeed while discovery cannot.</li><li>Confirm credentials, ONVIF availability and RTSP settings separately before diagnosing Live View.</li><li>Save observations and export the relevant diagnostics for the next visit.</li></ol>
<p>See <a href="/features.html#group-discover">discovery tools and screenshots</a>, or follow the <a href="/cctv-installer-software.html">full installation and handover workflow</a>.</p>''',
    },
    {
        "file": "cctv-installer-software.html",
        "title": "Free CCTV Installer Software & Camera Tools | SADPTools",
        "description": "CCTV tools for discovery, commissioning, Live View, diagnostics and site handover. Free installer software for Windows, Linux and Mac, with encrypted site transfers.",
        "heading": "CCTV installer software for the whole site visit",
        "intro": "SADPTools keeps camera discovery, supported configuration, Live View and handover information together. Carry the same practical CCTV tools on a Windows, Linux or Apple Mac laptop.",
        "image": "sites",
        "caption": "A saved site in SADPTools. Linux capture using demonstration equipment and site details.",
        "body": '''
<h2>1. Discover and identify the equipment</h2>
<p>Find compatible Hikvision equipment with SADP, use ONVIF where supported, add known camera IPs, or inspect an explicit routed network range. Compare the model, serial and MAC address before selecting a device. The <a href="/cctv-camera-discovery.html">camera discovery guide</a> explains which method fits each connection.</p>
<h2>2. Commission supported cameras deliberately</h2>
<p>Activate inactive devices, configure supported IPv4 or DHCP settings, and review planned batch operations before applying them. Device and firmware support is checked per action. A timeout is an unconfirmed outcome: rediscover and investigate before sending another change.</p>
<p>For Hikvision installers, the <a href="/sadp-tool.html">SADP guide</a> covers local discovery and the difference between finding a device and configuring it. Recovery tools use existing authorised methods, including supported security questions; they are not a password bypass.</p>
<h2>3. Check the picture while you keep working</h2>
<p>Live View opens in a separate window and starts playing automatically, leaving the main inventory available. Inspect supported stream information, image settings, focus and PTZ controls. Reachability, credentials, codecs and the camera's capabilities determine which controls work.</p>
<p>Use <a href="/features.html#feature-live-view">Live View screenshots</a> to see the interface. These tools help commission and diagnose cameras; SADPTools is not a replacement for a production NVR or video management system.</p>
<h2>4. Diagnose the network and retain useful evidence</h2>
<p>The toolkit includes connectivity and service checks, supported ONVIF and ISAPI inspection, recording metadata, event tools, network diagnostics and planning utilities. Use the <a href="/features.html">complete CCTV feature catalogue</a> to check each tool's requirements and limits. Calculated storage or bandwidth estimates are planning aids, not measurements of a live installation.</p>
<h2>5. Save a site and move it to another laptop</h2>
<p>Keep named sites, equipment, locations, notes and known credentials together. Export the entire site as an encrypted <code>.sadpsite</code> file and import it on another laptop. Choose a transfer passphrase and provide it separately to the authorised recipient.</p>
<p>NX Camera Info JSON imports can include cameras and known passwords. Full reports also retain NX Witness system, server, user, storage and licence information in encrypted storage. A read-only NX Witness screen appears for those imported sites; ordinary sites created through discovery have no empty server area. Exporting a site preserves this information for the next laptop.</p>
<h2>6. Prepare the handover</h2>
<p>Create commissioning reports, equipment labels and reviewed diagnostic bundles. Keep a portable site backup for follow-up work. Review any export before sharing it, particularly when it contains credentials, licence information or customer details.</p>
<h2>Choose your laptop, keep your workflow</h2>
<ul><li><strong>Windows 10/11 AMD64:</strong> an installer with the display runtime included, or a standalone ZIP for a machine with WebView2 already installed.</li><li><strong>Linux AMD64:</strong> DEB, RPM or a manual tar.gz installer with desktop-menu integration. GTK4 and WebKitGTK requirements apply.</li><li><strong>Apple Mac:</strong> a universal DMG or ZIP for Apple Silicon and Intel, targeting macOS 12 or later. Current bundles are not notarised; real Mac hardware testing remains necessary.</li></ul>
<p>SADPTools is free to use, with no subscription, app account or application telemetry. Site data stays on your computer. See <a href="/windows-help.html">Windows installation help</a>, <a href="/mac-help.html">Mac installation help</a> and the <a href="/privacy.html">privacy information</a>.</p>''',
    },
]


GUIDES.extend(TASK_GUIDES)


def guide_cards(guides=None):
    return '<div class="guide-cards">' + ''.join(
        f'<article><h3><a href="/{g["file"]}">{escape(g["heading"])}</a></h3><p>{escape(g["intro"])}</p></article>'
        for g in (GUIDES[:3] if guides is None else guides)
    ) + '</div>'


def render_guide(guide, base):
    g = guide
    related_names = g.get('related', [other['file'] for other in GUIDES[:3] if other != g])
    related = ''.join(f'<li><a href="/{other["file"]}">{escape(other["heading"])}</a></li>' for other in GUIDES if other['file'] in related_names)
    headings = []
    def section(match):
        title = match.group(1)
        anchor = re.sub(r'[^a-z0-9]+', '-', title.lower()).strip('-')
        headings.append((anchor, title))
        return f'<h2 id="{anchor}">{title}</h2>'
    content = re.sub(r'<h2>(.*?)</h2>', section, g['body'])
    contents = '<nav class="guide-contents" aria-label="On this page"><strong>On this page</strong><ul>' + ''.join(f'<li><a href="#{anchor}">{title}</a></li>' for anchor, title in headings) + '</ul></nav>'
    body = f'''<article class="guide-article"><header class="section guide-hero"><div class="container narrow">
<nav aria-label="Breadcrumb"><a href="/">SADPTools</a> / <a href="/guides.html">Installer guides</a></nav>
<p class="eyebrow">WINDOWS · LINUX · APPLE MAC</p><h1>{escape(g['heading'])}</h1>
<p class="section-intro">{escape(g['intro'])}</p><a class="button" href="/#downloads">Download SADPTools</a>
<p class="guide-byline">By <a href="https://www.paulwillard.nz/">Paul Willard</a>, creator of SADPTools</p>
<p class="guide-byline">Guide checked for SADPTools 1.0.1 · <time datetime="2026-10-28">28 October 2026</time>. Device and firmware support varies.</p>
</div></header><div class="container narrow prose guide-body">
{contents}
<figure class="showcase"><img class="app-shot-light" src="/assets/screens/{g['image']}-light.webp" width="1440" height="880" alt="{escape(g['caption'])}"><img class="app-shot-dark" src="/assets/screens/{g['image']}-dark.webp" width="1440" height="880" alt="{escape(g['caption'])}"><figcaption>{escape(g['caption'])}</figcaption></figure>
{content}<aside class="guide-related"><h2>Keep exploring</h2><ul>{related}<li><a href="/guides.html">All installation guides</a></li><li><a href="/features.html">All features and screenshots</a></li></ul><a class="button" href="/#downloads">Choose your free download</a></aside></div></article>'''
    schema = {"@context": "https://schema.org", "@graph": [
        {"@type": "TechArticle", "@id": base + '/' + g['file'] + '#article', "headline": g['heading'],
         "description": g['description'], "url": base + '/' + g['file'], "inLanguage": "en",
         "dateModified": "2026-10-28", "articleSection": "Camera installation guides",
         "mainEntityOfPage": {"@id": base + '/' + g['file'] + '#webpage'},
         "author": {"@type": "Person", "name": "Paul Willard", "url": "https://www.paulwillard.nz/"},
         "about": {"@id": base + '/#application'}, "image": base + '/assets/screens/' + g['image'] + '-light.webp'},
        {"@type": "BreadcrumbList", "itemListElement": [
            {"@type": "ListItem", "position": 1, "name": "SADPTools", "item": base + '/'},
            {"@type": "ListItem", "position": 2, "name": "Installer guides", "item": base + '/guides.html'},
            {"@type": "ListItem", "position": 3, "name": g['heading'], "item": base + '/' + g['file']},
        ]},
    ]}
    return body, schema


def render_guide_index(base):
    body = '''<section class="section guide-hero"><div class="container"><p class="eyebrow">WINDOWS · LINUX · APPLE MAC</p>
<h1>Camera installation guides</h1><p class="section-intro">Find the device, configure supported cameras and carry the installation to the next laptop. These guides explain the steps in SADPTools, what you need before starting and how to check the result.</p>
<p>SADPTools is independent software by Paul Willard. Hikvision compatibility depends on the device and firmware; it is not an official Hikvision product.</p></div></section>
<section class="section guide-index"><div class="container"><h2>Choose the job you are doing</h2>''' + guide_cards(GUIDES) + '''
<div class="guide-related"><h2>Installation help and downloads</h2><p><a href="/windows-help.html">Windows installation help</a> · <a href="/mac-help.html">Mac installation help</a> · <a href="/features.html">Full feature catalogue and screenshots</a></p><a class="button" href="/#downloads">Download SADPTools</a></div></div></section>'''
    schema = {'@context':'https://schema.org','@type':'CollectionPage','name':'SADPTools camera installation guides',
        'url':base+'/guides.html','about':{'@id':base+'/#application'},
        'mainEntity':{'@type':'ItemList','numberOfItems':len(GUIDES),'itemListElement':[
            {'@type':'ListItem','position':i,'name':g['heading'],'url':base+'/'+g['file']} for i,g in enumerate(GUIDES,1)]}}
    return body, schema
