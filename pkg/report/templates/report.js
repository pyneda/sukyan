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

  const payload = window.__SUKYAN_REPORT__ || { grouped_issues: [], summary: {} }

  const state = {
    groups: payload.grouped_issues || [],
    filtered: payload.grouped_issues || [],
    severity: 'all',
    query: '',
    selected: null,
  }

  function el(tag, className, text) {
    const node = document.createElement(tag)
    if (className) node.className = className
    if (text !== undefined && text !== null) node.textContent = String(text)
    return node
  }

  const SVG_NS = 'http://www.w3.org/2000/svg'

  function icon(paths, size = 14) {
    const svg = document.createElementNS(SVG_NS, 'svg')
    svg.setAttribute('width', String(size))
    svg.setAttribute('height', String(size))
    svg.setAttribute('viewBox', '0 0 24 24')
    svg.setAttribute('fill', 'none')
    svg.setAttribute('stroke', 'currentColor')
    svg.setAttribute('stroke-width', '2')
    svg.setAttribute('stroke-linecap', 'round')
    svg.setAttribute('stroke-linejoin', 'round')
    svg.setAttribute('aria-hidden', 'true')
    for (const d of paths) {
      const path = document.createElementNS(SVG_NS, 'path')
      path.setAttribute('d', d)
      svg.appendChild(path)
    }
    return svg
  }

  // Scanner output is attacker-influenced: a scanned application controls its
  // own URLs and payloads. Highlighting builds text nodes so a crafted title
  // cannot introduce markup.
  function highlight(text, query) {
    const fragment = document.createDocumentFragment()
    const value = String(text ?? '')

    if (!query) {
      fragment.appendChild(document.createTextNode(value))
      return fragment
    }

    const haystack = value.toLowerCase()
    const needle = query.toLowerCase()
    let index = 0

    for (;;) {
      const found = haystack.indexOf(needle, index)
      if (found === -1) break
      if (found > index) fragment.appendChild(document.createTextNode(value.slice(index, found)))
      fragment.appendChild(el('mark', 'mark', value.slice(found, found + needle.length)))
      index = found + needle.length
    }

    if (index < value.length) fragment.appendChild(document.createTextNode(value.slice(index)))
    return fragment
  }

  function severityClass(severity) {
    const known = ['Critical', 'High', 'Medium', 'Low', 'Info']
    return known.includes(severity) ? `badge badge-sev-${severity.toLowerCase()}` : 'badge badge-sev-info'
  }

  function flatIssues() {
    const all = []
    for (const group of state.filtered) {
      for (const issue of group.issues || []) {
        if (state.severity !== 'all' && issue.severity !== state.severity) continue
        all.push(issue)
      }
    }
    return all
  }

  function showToast(message) {
    const toast = document.getElementById('toast')
    if (!toast) return
    toast.textContent = message
    toast.style.opacity = '1'
    toast.style.transform = 'translateY(0)'
    clearTimeout(showToast.timer)
    showToast.timer = setTimeout(() => {
      toast.style.opacity = '0'
      toast.style.transform = 'translateY(0.5rem)'
    }, 2200)
  }

  function copyToClipboard(text) {
    if (!navigator.clipboard) {
      showToast('Clipboard unavailable')
      return
    }
    navigator.clipboard.writeText(text).then(
      () => showToast('Copied to clipboard'),
      () => showToast('Copy failed'),
    )
  }

  // Replaces Fuse.js. Exact substring beats token-prefix; title beats code
  // beats url. Enough for a few thousand groups, and it costs nothing to ship.
  function scoreGroup(group, query) {
    if (!query) return 1

    const fields = [
      [String(group.title || '').toLowerCase(), 3],
      [String(group.code || '').toLowerCase(), 2],
    ]
    for (const issue of group.issues || []) {
      fields.push([String(issue.url || '').toLowerCase(), 1])
    }

    const needle = query.toLowerCase()
    const tokens = needle.split(/\s+/).filter(Boolean)
    let score = 0

    for (const [value, weight] of fields) {
      if (value.includes(needle)) {
        score += weight * 10
        continue
      }
      for (const token of tokens) {
        if (value.includes(token)) score += weight
      }
    }

    return score
  }

  function updateResultCount() {
    const node = document.getElementById('result-count')
    if (!node) return
    const count = flatIssues().length
    node.textContent = `${count} ${count === 1 ? 'issue' : 'issues'}`
  }

  function applyFilters() {
    const query = state.query.trim()

    const matched = state.groups
      .map((group) => ({ group, score: scoreGroup(group, query) }))
      .filter((entry) => entry.score > 0)
      .filter(
        (entry) =>
          state.severity === 'all' ||
          (entry.group.issues || []).some((issue) => issue.severity === state.severity),
      )

    if (query) matched.sort((a, b) => b.score - a.score)

    state.filtered = matched.map((entry) => entry.group)
    renderList()
    updateResultCount()
  }

  const CHEVRON = ['M9 18l6-6-6-6']

  function buildIssueRow(issue) {
    const row = el('button', 'issue-row')
    row.type = 'button'
    row.dataset.issueId = String(issue.id)

    const title = el('div', 'issue-row-title')
    title.appendChild(highlight(issue.title, state.query))
    row.appendChild(title)

    if (issue.url) {
      const url = el('div', 'issue-row-url')
      url.appendChild(highlight(issue.url, state.query))
      row.appendChild(url)
    }

    const meta = el('div', 'issue-row-meta')
    meta.appendChild(el('span', severityClass(issue.severity), issue.severity))
    meta.appendChild(el('span', 'badge badge-outline', `${issue.confidence}%`))
    if (issue.false_positive) meta.appendChild(el('span', 'badge badge-outline', 'False positive'))
    row.appendChild(meta)

    row.addEventListener('click', () => selectIssue(issue))
    return row
  }

  // Children are built on first expand. Building every card for every group up
  // front is what made large reports slow to open.
  function fillGroupBody(group, body) {
    if (body.dataset.filled === 'true') return
    const inner = el('div')
    for (const issue of group.issues || []) {
      if (state.severity !== 'all' && issue.severity !== state.severity) continue
      inner.appendChild(buildIssueRow(issue))
    }
    body.appendChild(inner)
    body.dataset.filled = 'true'
  }

  function renderList() {
    const container = document.getElementById('issues-container')
    if (!container) return
    container.replaceChildren()

    if (state.filtered.length === 0) {
      container.appendChild(el('p', 'empty', 'No issues match your filters'))
      return
    }

    state.filtered.forEach((group, index) => {
      const wrapper = el('div', 'group')
      wrapper.dataset.open = index === 0 ? 'true' : 'false'

      const head = el('button', 'group-head')
      head.type = 'button'
      head.setAttribute('aria-expanded', wrapper.dataset.open)

      const chevron = icon(CHEVRON, 12)
      chevron.classList.add('group-chevron')
      head.appendChild(chevron)

      const visible = (group.issues || []).filter(
        (issue) => state.severity === 'all' || issue.severity === state.severity,
      ).length

      const label = el('span', null)
      label.style.flex = '1'
      label.style.minWidth = '0'
      label.appendChild(highlight(group.title, state.query))
      head.appendChild(label)
      head.appendChild(el('span', severityClass(group.severity), group.severity))
      head.appendChild(el('span', 'badge badge-outline', String(visible)))

      const body = el('div', 'group-body')

      head.addEventListener('click', () => {
        const open = wrapper.dataset.open !== 'true'
        wrapper.dataset.open = String(open)
        head.setAttribute('aria-expanded', String(open))
        if (open) fillGroupBody(group, body)
      })

      wrapper.appendChild(head)
      wrapper.appendChild(body)
      if (index === 0) fillGroupBody(group, body)

      container.appendChild(wrapper)
    })

    // A re-render rebuilds the rows, so the selection marker has to be put back.
    if (state.selected) markSelectedRow(state.selected.id)
  }

  function markSelectedRow(issueId) {
    for (const row of document.querySelectorAll('.issue-row-selected')) {
      row.classList.remove('issue-row-selected')
    }
    const row = document.querySelector(`.issue-row[data-issue-id="${issueId}"]`)
    if (row) {
      row.classList.add('issue-row-selected')
      row.scrollIntoView({ block: 'nearest' })
    }
  }

  function selectIssue(issue) {
    if (!issue) return
    state.selected = issue
    renderDetail(issue)
    markSelectedRow(issue.id)

    const hash = `#issue-${issue.id}`
    if (window.location.hash !== hash) {
      history.replaceState(null, '', hash)
    }
  }

  function issueById(id) {
    for (const group of state.groups) {
      for (const issue of group.issues || []) {
        if (String(issue.id) === String(id)) return issue
      }
    }
    return null
  }

  // Expanding the owning group first, so the row exists to be highlighted.
  function revealIssue(issue) {
    const index = state.filtered.findIndex((group) =>
      (group.issues || []).some((candidate) => candidate.id === issue.id),
    )
    if (index === -1) return
    const wrapper = document.querySelectorAll('#issues-container .group')[index]
    if (!wrapper) return
    if (wrapper.dataset.open !== 'true') {
      wrapper.querySelector('.group-head').click()
    }
  }

  function moveSelection(delta) {
    const all = flatIssues()
    if (all.length === 0) return
    const current = all.findIndex((issue) => state.selected && issue.id === state.selected.id)
    const next = all[Math.min(all.length - 1, Math.max(0, current + delta))]
    if (!next) return
    revealIssue(next)
    selectIssue(next)
  }

  function bindControls() {
    const search = document.getElementById('search-input')
    if (search) {
      let timer
      search.addEventListener('input', (event) => {
        const value = event.target.value
        clearTimeout(timer)
        timer = setTimeout(() => {
          state.query = value
          applyFilters()
        }, 120)
      })
    }

    const filter = document.getElementById('severity-filter')
    if (filter) {
      filter.addEventListener('change', (event) => {
        state.severity = event.target.value
        applyFilters()
      })
    }

    document.addEventListener('keydown', (event) => {
      const typing =
        event.target instanceof HTMLInputElement || event.target instanceof HTMLSelectElement

      if (event.key === '/' && !typing) {
        event.preventDefault()
        search?.focus()
        return
      }
      if (event.key === 'Escape' && typing) {
        event.target.blur()
        return
      }
      if (typing) return

      if (event.key === 'ArrowDown') {
        event.preventDefault()
        moveSelection(1)
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        moveSelection(-1)
      }
    })
  }

  function renderDetail(issue) {
    const container = document.getElementById('issue-details')
    if (!container) return
    container.replaceChildren()

    const head = el('div', 'card-head')
    head.style.alignItems = 'flex-start'

    const left = el('div')
    left.style.minWidth = '0'
    const title = el('h2', null, issue.title)
    title.style.fontSize = '1rem'
    title.style.fontWeight = '600'
    left.appendChild(title)

    const meta = el('div')
    meta.style.display = 'flex'
    meta.style.flexWrap = 'wrap'
    meta.style.gap = '0.375rem'
    meta.style.marginTop = '0.375rem'
    if (issue.http_method) meta.appendChild(el('span', 'chip', issue.http_method))
    if (issue.status_code) meta.appendChild(el('span', 'chip', String(issue.status_code)))
    if (issue.cwe) meta.appendChild(el('span', 'chip', `CWE-${issue.cwe}`))
    if (issue.created_at) meta.appendChild(el('span', 'chip', issue.created_at))
    left.appendChild(meta)
    head.appendChild(left)

    const right = el('div')
    right.style.display = 'flex'
    right.style.alignItems = 'center'
    right.style.gap = '0.375rem'
    right.appendChild(el('span', severityClass(issue.severity), String(issue.severity).toUpperCase()))
    right.appendChild(el('span', 'badge badge-outline', `${issue.confidence}% confidence`))
    head.appendChild(right)

    container.appendChild(head)

    const body = el('div', 'card-body')

    if (issue.false_positive) {
      const banner = el('div', 'field')
      const notice = el('p', null, 'This finding has been marked as a false positive.')
      notice.style.padding = '0.625rem 0.75rem'
      notice.style.border = '1px solid var(--severity-medium)'
      notice.style.borderRadius = 'var(--radius)'
      notice.style.fontSize = '0.8125rem'
      banner.appendChild(notice)
      body.appendChild(banner)
    }

    body.appendChild(buildDetailBody(issue))
    container.appendChild(body)
  }

  // Replaced by the tabbed implementation.
  function buildDetailBody(issue) {
    const panel = el('div')
    if (issue.description) {
      panel.appendChild(el('p', 'field-label', 'Summary'))
      panel.appendChild(el('p', 'field-value', issue.description))
    }
    return panel
  }

  function bootstrap() {
    bindTheme()
    bindControls()
    applyFilters()

    const fromHash = window.location.hash.startsWith('#issue-')
      ? issueById(window.location.hash.slice('#issue-'.length))
      : null

    // Opening on the worst finding rather than an empty pane: the largest
    // region of the page should say something on first paint.
    const initial = fromHash || flatIssues()[0]
    if (initial) {
      revealIssue(initial)
      selectIssue(initial)
    }
  }

  document.addEventListener('DOMContentLoaded', bootstrap)
})()
