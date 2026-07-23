(function () {
  var dropdowns = document.querySelectorAll('.header-dropdown');
  var lastToggle = null; // tracks which button opened a panel

  function closeAll(returnFocus) {
    dropdowns.forEach(function (dd) {
      var panel = dd.querySelector('.header-dropdown-panel');
      var toggle = dd.querySelector('.header-dropdown-toggle');
      if (panel) panel.classList.remove('open');
      if (toggle) toggle.setAttribute('aria-expanded', 'false');
    });
    if (returnFocus && lastToggle) {
      lastToggle.focus();
      lastToggle = null;
    }
  }

  function openDropdown(dd) {
    closeAll(false);
    // Also close sidebar if open
    var sidebar = document.querySelector('.sidebar.open');
    if (sidebar) {
      sidebar.classList.remove('open');
      var backdrop = document.querySelector('.sidebar-backdrop');
      if (backdrop) backdrop.classList.remove('open');
      var header = document.querySelector('.site-header');
      var main = document.querySelector('.content');
      if (header) header.inert = false;
      if (main) main.inert = false;
    }

    var panel = dd.querySelector('.header-dropdown-panel');
    var toggle = dd.querySelector('.header-dropdown-toggle');
    if (panel) panel.classList.add('open');
    if (toggle) {
      toggle.setAttribute('aria-expanded', 'true');
      lastToggle = toggle;
    }
  }

  function isOpen(dd) {
    var panel = dd.querySelector('.header-dropdown-panel');
    return panel && panel.classList.contains('open');
  }

  function anyOpen() {
    for (var i = 0; i < dropdowns.length; i++) {
      if (isOpen(dropdowns[i])) return true;
    }
    return false;
  }

  function focusSearchInput() {
    var searchEl = document.querySelector('alloy-search');
    if (searchEl && searchEl.shadowRoot) {
      var input = searchEl.shadowRoot.querySelector('input[type="search"]');
      if (input) requestAnimationFrame(function () { input.focus(); });
    }
  }

  // Toggle buttons
  dropdowns.forEach(function (dd) {
    var toggle = dd.querySelector('.header-dropdown-toggle');
    if (!toggle) return;

    toggle.addEventListener('click', function (e) {
      e.stopPropagation();
      if (isOpen(dd)) {
        closeAll(true);
      } else {
        openDropdown(dd);
        if (dd.id === 'search-dropdown') focusSearchInput();
      }
    });
  });

  // Click outside closes all — no focus return (focus goes to click target)
  document.addEventListener('click', function (e) {
    if (!anyOpen()) return;
    var inside = false;
    var composed = e.composedPath ? e.composedPath() : [e.target];
    for (var i = 0; i < composed.length; i++) {
      for (var j = 0; j < dropdowns.length; j++) {
        if (composed[i] === dropdowns[j]) { inside = true; break; }
      }
      if (inside) break;
    }
    if (!inside) closeAll(false);
  });

  // Escape closes all and returns focus to toggle
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && anyOpen()) {
      e.stopPropagation();
      closeAll(true);
    }
  });

  // Tab out of the open panel closes it
  document.addEventListener('focusin', function (e) {
    if (!anyOpen()) return;
    for (var i = 0; i < dropdowns.length; i++) {
      if (!isOpen(dropdowns[i])) continue;
      var inside = dropdowns[i].contains(e.target);
      // Check shadow DOM
      if (!inside && e.composedPath) {
        var path = e.composedPath();
        for (var j = 0; j < path.length; j++) {
          if (path[j] === dropdowns[i]) { inside = true; break; }
        }
      }
      if (!inside) closeAll(false);
      break;
    }
  });

  // "/" opens search dropdown on mobile
  document.addEventListener('keydown', function (e) {
    if (e.key !== '/') return;
    if (['INPUT', 'TEXTAREA', 'SELECT'].indexOf(document.activeElement.tagName) >= 0) return;
    var searchDD = document.getElementById('search-dropdown');
    if (!searchDD) return;
    var toggle = searchDD.querySelector('.header-dropdown-toggle');
    // Only intercept when toggle is visible (narrow container); desktop handled by alloy-search
    if (!toggle || getComputedStyle(toggle).display === 'none') return;
    e.preventDefault();
    openDropdown(searchDD);
    focusSearchInput();
  });

  // Close dropdowns when sidebar hamburger is clicked
  var menuToggle = document.querySelector('.menu-toggle');
  if (menuToggle) menuToggle.addEventListener('click', function () { closeAll(false); });
})();
