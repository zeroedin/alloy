(function () {
  var toggle = document.querySelector('.menu-toggle');
  var sidebar = document.querySelector('.sidebar');
  var closeBtn = document.querySelector('.sidebar-close');
  var backdrop = document.querySelector('.sidebar-backdrop');
  var header = document.querySelector('.site-header');
  var main = document.querySelector('.content');
  if (!toggle || !sidebar) return;

  function open() {
    sidebar.classList.add('open');
    if (backdrop) backdrop.classList.add('open');
    toggle.setAttribute('aria-expanded', 'true');
    if (header) header.inert = true;
    if (main) main.inert = true;
    if (closeBtn) closeBtn.focus();
  }

  function close() {
    sidebar.classList.remove('open');
    if (backdrop) backdrop.classList.remove('open');
    toggle.setAttribute('aria-expanded', 'false');
    if (header) header.inert = false;
    if (main) main.inert = false;
    toggle.focus();
  }

  toggle.addEventListener('click', open);
  if (closeBtn) closeBtn.addEventListener('click', close);
  if (backdrop) backdrop.addEventListener('click', close);
  sidebar.querySelectorAll('a').forEach(function (a) { a.addEventListener('click', close); });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && sidebar.classList.contains('open')) close();
  });
})();
