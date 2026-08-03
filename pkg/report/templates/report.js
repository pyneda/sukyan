;(() => {
  const THEME_KEY = 'sukyan-report-theme'

  function applyTheme(theme) {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    const button = document.getElementById('theme-toggle')
    if (button) {
      button.setAttribute('aria-label', theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme')
      button.textContent = theme === 'dark' ? 'Light' : 'Dark'
    }
  }

  function storedTheme() {
    try {
      return localStorage.getItem(THEME_KEY)
    } catch {
      return null
    }
  }

  applyTheme(storedTheme() || 'dark')

  function bindTheme() {
    applyTheme(storedTheme() || 'dark')

    const button = document.getElementById('theme-toggle')
    if (!button) return

    button.addEventListener('click', () => {
      const next = document.documentElement.classList.contains('dark') ? 'light' : 'dark'
      try {
        localStorage.setItem(THEME_KEY, next)
      } catch {
        // A report opened over file:// may have no storage. The toggle still
        // works for the session.
      }
      applyTheme(next)
    })
  }

  document.addEventListener('DOMContentLoaded', () => {
    bindTheme()
  })
})()
