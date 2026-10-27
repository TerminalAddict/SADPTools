(() => {
  const root = document.documentElement;
  const themeButton = document.querySelector('.theme-toggle');
  function themeLabel() {
    const dark = root.dataset.theme === 'dark';
    themeButton.setAttribute('aria-label', `Switch to ${dark ? 'light' : 'dark'} mode`);
    themeButton.setAttribute('title', `Switch to ${dark ? 'light' : 'dark'} mode`);
    themeButton.querySelector('use').setAttribute('href', `/assets/icons.svg#${dark ? 'sun' : 'moon'}`);
    document.querySelector('.theme-label').textContent = dark ? 'Light mode' : 'Dark mode';
  }
  themeLabel();
  themeButton.addEventListener('click', () => {
    root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem('sadptools-theme', root.dataset.theme); } catch {}
    themeLabel();
  });
  const menu = document.querySelector('.menu-toggle');
  menu.addEventListener('click', () => menu.setAttribute('aria-expanded', String(menu.getAttribute('aria-expanded') !== 'true')));
  document.querySelectorAll('#navigation a').forEach(a => a.addEventListener('click', () => menu.setAttribute('aria-expanded', 'false')));
  document.querySelectorAll('[data-preview-theme]').forEach(button => button.addEventListener('click', () => {
    const theme = button.dataset.previewTheme;
    document.querySelectorAll('[data-preview-theme]').forEach(b => b.setAttribute('aria-pressed', String(b === button)));
    const image = document.querySelector('#preview-image');
    image.src = `/assets/app-${theme}.png`;
    image.alt = `Actual SADPTools ${theme}-theme application screenshot with synthetic demonstration site data`;
  }));
  const status = document.querySelector('#release-status');
  if (!status) return;
  fetch('/api/releases', {signal: AbortSignal.timeout(15000), credentials: 'omit'})
    .then(r => { if (!r.ok) throw new Error('unavailable'); return r.json(); })
    .then(data => {
      if (!/^\d+\.\d+\.\d+(?:-(?:beta|rc)\.\d+)?$/.test(data.version)) throw new Error('version');
      const label = data.version.includes('-rc.') ? 'Current release candidate' : data.prerelease ? 'Current beta' : 'Current release';
      status.textContent = `${label} · ${data.version}${data.stale ? ' · last confirmed release (refresh pending)' : ''}`;
      for (const [key, asset] of Object.entries(data.assets)) {
        const link = document.querySelector(`[data-download="${CSS.escape(key)}"]`);
        if (link) link.title = `${asset.name} · ${(asset.size / 1048576).toFixed(1)} MB`;
        const size = document.querySelector(`[data-size="${CSS.escape(key)}"]`);
        if (size) size.textContent = `${(asset.size / 1048576).toFixed(1)} MB · ${size.dataset.format || "Download"} · ${data.version}`;
      }
    }).catch(() => { status.textContent = 'Release information is temporarily unavailable. Please try the download again shortly.'; });
  document.querySelectorAll('.download-link').forEach(link => link.addEventListener('click', () => {
    document.querySelector('#download-feedback').textContent = 'Your download is being prepared. The first request may take a moment while the file is verified.';
  }));
})();
