(() => {
  const form = document.querySelector('#record-selection');
  if (!form) return;
  const rows = [...document.querySelectorAll('[data-record-checkbox]')];
  const page = document.querySelector('#select-page');
  const all = document.querySelector('#select-matching');
  const status = document.querySelector('#selection-status');
  const remove = document.querySelector('#delete-selected');
  const clear = document.querySelector('#clear-selection');
  const excluded = document.querySelector('#excluded-records');
  const matches = Number(form.dataset.matches);
  page.disabled = false;
  clear.hidden = false;
  const update = () => {
    const checked = rows.filter(row => row.checked);
    const count = all.checked ? matches - (rows.length - checked.length) : checked.length;
    page.checked = checked.length === rows.length;
    page.indeterminate = checked.length > 0 && checked.length < rows.length;
    remove.disabled = count === 0;
    status.textContent = `${count.toLocaleString()} selected ${all.checked ? 'across all pages' : 'on this page'}`;
    excluded.replaceChildren();
    for (const row of rows) {
      row.closest('tr').classList.toggle('is-selected', row.checked);
      if (all.checked && !row.checked) {
        const input = document.createElement('input');
        input.type = 'hidden'; input.name = 'excluded[]'; input.value = row.value;
        excluded.append(input);
      }
    }
  };
  all.addEventListener('change', () => { rows.forEach(row => row.checked = all.checked); update(); });
  page.addEventListener('change', () => { rows.forEach(row => row.checked = page.checked); update(); });
  rows.forEach(row => row.addEventListener('change', update));
  clear.addEventListener('click', () => { all.checked = false; rows.forEach(row => row.checked = false); update(); });
  // Never restore a stale selection through back/forward navigation.
  window.addEventListener('pageshow', () => { all.checked = false; rows.forEach(row => row.checked = false); update(); });
  update();
})();
