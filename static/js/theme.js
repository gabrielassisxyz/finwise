(function () {
  'use strict';

  const STORAGE_KEY = 'finwise-theme';
  const THEMES = ['finwise', 'finwiseSoft', 'finwiseOled'];

  function getSavedTheme() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved && THEMES.includes(saved)) {
        return saved;
      }
    } catch (e) {
      // localStorage may be unavailable
    }
    return THEMES[0];
  }

  function setTheme(name) {
    if (!THEMES.includes(name)) return;
    document.documentElement.setAttribute('data-theme', name);
    try {
      localStorage.setItem(STORAGE_KEY, name);
    } catch (e) {
      // localStorage may be unavailable
    }
    window.dispatchEvent(new CustomEvent('finwise:themechange', { detail: { theme: name } }));
  }

  function getNextTheme() {
    const current = document.documentElement.getAttribute('data-theme') || THEMES[0];
    const idx = THEMES.indexOf(current);
    return THEMES[(idx + 1) % THEMES.length];
  }

  function toggleTheme() {
    setTheme(getNextTheme());
  }

  function init() {
    const saved = getSavedTheme();
    document.documentElement.setAttribute('data-theme', saved);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  window.FinWiseTheme = {
    set: setTheme,
    get: () => document.documentElement.getAttribute('data-theme') || THEMES[0],
    toggle: toggleTheme,
    next: getNextTheme,
    themes: THEMES.slice()
  };
})();
