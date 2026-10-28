"""Keep sitemap dates tied to actual page changes, not rebuilds or pings."""
from datetime import datetime, timezone
import hashlib
import json
from xml.etree import ElementTree as ET
from discovery_guides import GUIDES

PUBLIC_PAGES = ('index.html', 'features.html', 'guides.html', *(g['file'] for g in GUIDES),
                'windows-help.html', 'mac-help.html', 'privacy.html')
NS = 'http://www.sitemaps.org/schemas/sitemap/0.9'


def write_discovery_files(root, base):
    state_path = root.parent / 'content/page-updates.json'
    previous = json.loads(state_path.read_text()) if state_path.exists() else {}
    now = datetime.now(timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')
    state = {}
    ET.register_namespace('', NS)
    sitemap = ET.Element('{' + NS + '}urlset')
    for name in PUBLIC_PAGES:
        digest = hashlib.sha256((root / name).read_bytes()).hexdigest()
        old = previous.get(name, {})
        modified = old['lastmod'] if old.get('sha256') == digest else now
        state[name] = {'sha256': digest, 'lastmod': modified}
        item = ET.SubElement(sitemap, 'url')
        ET.SubElement(item, 'loc').text = base + ('/' if name == 'index.html' else '/' + name)
        ET.SubElement(item, 'lastmod').text = modified
    ET.indent(sitemap, space='  ')
    (root / 'sitemap.xml').write_bytes(ET.tostring(sitemap, encoding='utf-8', xml_declaration=True) + b'\n')
    state_path.write_text(json.dumps(state, indent=2) + '\n')
    exclusions = 'Disallow: /_app/\nDisallow: /download/\nDisallow: /download.php\nDisallow: /api/\nDisallow: /assets/private-downloads/\nDisallow: /assets/downloads_report.php\n'
    (root / 'robots.txt').write_text('# Public guides are available to search crawlers. Private paths stay excluded.\n'
        'User-agent: *\n' + exclusions + '\n# ChatGPT search: same public access and private exclusions.\n'
        'User-agent: OAI-SearchBot\n' + exclusions + '\nSitemap: ' + base + '/sitemap.xml\n')
