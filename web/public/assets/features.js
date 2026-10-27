(() => {
  const cards = [...document.querySelectorAll('[data-feature]')];
  const groups = [...document.querySelectorAll('.feature-group')];
  const search = document.querySelector('#feature-search');
  const count = document.querySelector('#feature-count');
  const empty = document.querySelector('#feature-empty');
  const viewer = document.querySelector('#screen-viewer');
  const image = document.querySelector('#viewer-image');
  const caption = document.querySelector('#viewer-caption');
  const title = document.querySelector('#viewer-title');
  const original = document.querySelector('#viewer-original');
  let selected = 0;
  let viewerTheme = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
  const normalise = value => value.normalize('NFKD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
  const searchable = cards.map(card => normalise(card.textContent));
  const visible = () => cards.filter(card => !card.hidden);
  const screenPath = (card, theme, ext) => `/assets/screens/${card.dataset.feature}-${theme}.${ext}?v=${card.dataset[`${theme}Version`]}`;
  function thumbnails() {
    const theme = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
    for (const card of cards) {
      const thumb = card.querySelector('[data-screen]');
      thumb.src = screenPath(card, theme, 'webp');
      card.querySelector('[data-gallery]').href = screenPath(card, theme, 'png');
    }
  }
  thumbnails();
  new MutationObserver(thumbnails).observe(document.documentElement, {attributes: true, attributeFilter: ['data-theme']});
  function filter() {
    const terms = normalise(search.value.trim()).split(/\s+/).filter(Boolean);
    cards.forEach((card, i) => { card.hidden = !terms.every(term => searchable[i].includes(term)); });
    groups.forEach(group => { group.hidden = ![...group.querySelectorAll('[data-feature]')].some(card => !card.hidden); });
    const n = visible().length;
    count.textContent = terms.length ? `${n} of ${cards.length} feature areas` : `${cards.length} feature areas · ${groups.length} categories`;
    empty.hidden = n > 0;
  }
  search.addEventListener('input', filter);
  document.querySelector('#clear-feature-search').addEventListener('click', () => { search.value = ''; filter(); search.focus(); });
  document.querySelectorAll('.feature-sidebar nav a').forEach(link => link.addEventListener('click', () => { search.value = ''; filter(); }));
  function renderViewer() {
    const list = visible();
    const card = list[selected];
    if (!card) { viewer.close(); return; }
    const label = card.querySelector('h3').textContent;
    title.textContent = label;
    caption.textContent = card.querySelector('[data-caption]').textContent;
    document.querySelector('#viewer-counter').textContent = `SCREEN ${selected + 1} OF ${list.length} · ${card.dataset.capture.toUpperCase()}`;
    image.alt = `${label} — ${viewerTheme} theme, ${card.dataset.capture.toLowerCase()} screen`;
    image.src = screenPath(card, viewerTheme, 'png');
    original.href = image.src;
    document.querySelectorAll('[data-viewer-theme]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.viewerTheme === viewerTheme)));
  }
  cards.forEach(card => card.querySelector('[data-gallery]').addEventListener('click', event => {
    // Preserve normal browser actions and a useful no-JavaScript full-image link.
    if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    selected = visible().indexOf(card);
    viewerTheme = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
    renderViewer();
    viewer.showModal();
    document.querySelector('#viewer-close').focus();
  }));
  const move = delta => { selected = (selected + delta + visible().length) % visible().length; renderViewer(); };
  document.querySelector('#viewer-close').addEventListener('click', () => viewer.close());
  document.querySelector('#viewer-prev').addEventListener('click', () => move(-1));
  document.querySelector('#viewer-next').addEventListener('click', () => move(1));
  document.querySelectorAll('[data-viewer-theme]').forEach(button => button.addEventListener('click', () => { viewerTheme = button.dataset.viewerTheme; renderViewer(); }));
  viewer.addEventListener('keydown', event => {
    if (event.key === 'ArrowLeft') { event.preventDefault(); move(-1); }
    if (event.key === 'ArrowRight') { event.preventDefault(); move(1); }
  });
  viewer.addEventListener('click', event => {
    const rect = viewer.getBoundingClientRect();
    if (event.target === viewer && (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom)) viewer.close();
  });
})();
