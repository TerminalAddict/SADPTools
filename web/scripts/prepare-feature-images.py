#!/usr/bin/env python3
"""Publish reviewed native captures and compact WebP previews.

Raw captures remain ignored. Run after visual/privacy review. PNGs retain the
actual captured pixels; settings is captured as a dialog detail in X11.
"""
from pathlib import Path
import hashlib
import json
import shutil
import subprocess

web = Path(__file__).resolve().parents[1]
raw = web / '.cache/feature-capture/raw'
out = web / 'public/assets/screens'
out.mkdir(parents=True, exist_ok=True)
catalogue = json.loads((web / 'content/features.json').read_text())
manifest = {}
for group in catalogue['groups']:
    for item in group['features']:
        name = item['id']
        hashes = {}
        camera = item.get('capture') == 'real-camera'
        source_dir = web / '.cache/live-camera-capture' if camera else raw
        for theme in ('light', 'dark'):
            source = source_dir / f'{name}-{theme}.png'
            assert source.is_file(), f'Missing capture: {source.name}'
            width, height = map(int, subprocess.check_output(['magick','identify','-format','%w %h',str(source)],text=True).split())
            if theme == 'light':
                origin = 'Native released Wails app with authorised physical-camera video via an ephemeral loopback relay; session-only credentials; no device writes' if camera else 'Native Wails app with synthetic screenshot fixtures; no physical-device validation'
                manifest[name] = {'width': width, 'height': height, 'caption': item['caption'], 'origin': origin}
            else:
                assert (width,height)==(manifest[name]['width'],manifest[name]['height']), f'Theme dimensions differ: {name}'
            shutil.copyfile(source, out/source.name)
            subprocess.run(['magick',str(source),'-resize','800x600>','-strip','-quality','86',str(out/f'{name}-{theme}.webp')],check=True)
            hashes[theme] = hashlib.sha256(source.read_bytes()).hexdigest()
        assert hashes['light'] != hashes['dark'], f'Themes unexpectedly identical: {name}'
        manifest[name]['sha256'] = hashes
(web/'content/screenshots.json').write_text(json.dumps(manifest,indent=2,ensure_ascii=False)+'\n')
print(f'Prepared {len(manifest)*2} full-resolution screenshots and {len(manifest)*2} WebP previews.')
