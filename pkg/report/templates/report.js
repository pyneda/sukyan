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

  // Reset per selection, so the print hook only ever builds panels that are
  // actually in the document.
  let panelBuilders = []

  function renderDetail(issue) {
    const container = document.getElementById('issue-details')
    if (!container) return
    container.replaceChildren()
    panelBuilders = []

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

  const COPY_ICON = [
    'M8 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2v-2',
    'M16 2H10a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2z',
  ]

  function decodeBase64(value) {
    if (!value) return ''
    try {
      // atob yields a binary string; this round-trip recovers UTF-8 text.
      return new TextDecoder().decode(Uint8Array.from(atob(value), (c) => c.charCodeAt(0)))
    } catch {
      return '[unable to decode content]'
    }
  }

  function formatBytes(count) {
    if (count < 1024) return `${count} B`
    if (count < 1024 * 1024) return `${(count / 1024).toFixed(1)} KB`
    return `${(count / (1024 * 1024)).toFixed(1)} MB`
  }

  function codeBlock(title, content, options = {}) {
    const block = el('div', 'code')

    const head = el('div', 'code-head')
    head.appendChild(el('span', null, title))

    const right = el('div')
    right.style.display = 'flex'
    right.style.alignItems = 'center'
    right.style.gap = '0.375rem'

    if (options.truncated) right.appendChild(el('span', 'badge badge-outline', 'Truncated'))
    right.appendChild(el('span', 'chip', formatBytes(new Blob([content]).size)))

    const copy = el('button', 'btn btn-ghost no-print')
    copy.type = 'button'
    copy.appendChild(icon(COPY_ICON, 12))
    copy.appendChild(el('span', null, 'Copy'))
    copy.addEventListener('click', () => copyToClipboard(content))
    right.appendChild(copy)

    head.appendChild(right)
    block.appendChild(head)
    block.appendChild(el('pre', 'code-body', content))
    return block
  }

  function field(label, value) {
    const wrapper = el('div', 'field')
    wrapper.appendChild(el('p', 'field-label', label))
    wrapper.appendChild(el('p', 'field-value', value))
    return wrapper
  }

  function separator() {
    return el('div', 'separator')
  }

  function labelledRow(label, value, labelWidth) {
    const row = el('div')
    row.style.display = 'flex'
    row.style.gap = '0.5rem'
    const key = el('span', 'stat-hint', label)
    key.style.minWidth = labelWidth
    key.style.flexShrink = '0'
    row.appendChild(key)
    const val = el('span', null, value)
    val.style.wordBreak = 'break-all'
    row.appendChild(val)
    return row
  }

  // Panels are built lazily on first activation, so opening a finding does not
  // pay for decoding evidence the reader may never look at.
  function buildTabs(definitions) {
    const wrapper = el('div')
    const list = el('div', 'tabs')
    list.setAttribute('role', 'tablist')
    const panels = el('div')

    definitions.forEach((definition, index) => {
      const tab = el('button', 'tab', definition.label)
      tab.type = 'button'
      tab.setAttribute('role', 'tab')
      tab.setAttribute('aria-selected', String(index === 0))

      const panel = el('div', 'tab-panel')
      panel.setAttribute('role', 'tabpanel')
      panel.hidden = index !== 0

      const build = () => {
        if (panel.dataset.built === 'true') return
        panel.appendChild(definition.build())
        panel.dataset.built = 'true'
      }

      if (index === 0) build()

      // Printing hides the tab strip and reveals every panel, so panels the
      // reader never opened still have to exist on paper.
      panelBuilders.push(build)

      tab.addEventListener('click', () => {
        for (const other of list.children) other.setAttribute('aria-selected', 'false')
        for (const other of panels.children) other.hidden = true
        tab.setAttribute('aria-selected', 'true')
        build()
        panel.hidden = false
      })

      list.appendChild(tab)
      panels.appendChild(panel)
    })

    wrapper.appendChild(list)
    wrapper.appendChild(panels)
    return wrapper
  }

  function linkTo(url) {
    const link = el('a', null, url)
    link.href = url
    link.target = '_blank'
    link.rel = 'noopener noreferrer'
    link.style.wordBreak = 'break-all'
    return link
  }

  function descriptionPanel(issue) {
    const panel = el('div')

    const urlField = el('div', 'field')
    urlField.appendChild(el('p', 'field-label', 'URL'))
    if (/^https?:\/\//i.test(issue.url || '')) {
      urlField.appendChild(linkTo(issue.url))
    } else {
      urlField.appendChild(el('p', 'field-value', issue.url || '-'))
    }
    panel.appendChild(urlField)

    if (issue.payload) {
      const payloadField = el('div', 'field')
      payloadField.appendChild(el('p', 'field-label', 'Payload'))
      payloadField.appendChild(codeBlock('Payload', issue.payload))
      panel.appendChild(payloadField)
    }

    panel.appendChild(separator())

    if (issue.description) panel.appendChild(field('Summary', issue.description))
    if (issue.details) panel.appendChild(field('Details', issue.details))
    if (issue.note) panel.appendChild(field('Notes', issue.note))

    if (issue.curl_command) {
      const curlField = el('div', 'field')
      curlField.appendChild(el('p', 'field-label', 'cURL Command'))
      curlField.appendChild(codeBlock('cURL', issue.curl_command))
      panel.appendChild(curlField)
    }

    if (issue.remediation) {
      panel.appendChild(separator())
      panel.appendChild(field('Remediation', issue.remediation))
    }

    if (issue.references && issue.references.length > 0) {
      const refField = el('div', 'field')
      refField.appendChild(el('p', 'field-label', 'References'))
      const list = el('ul')
      list.style.paddingLeft = '1.125rem'
      for (const reference of issue.references) {
        const item = el('li')
        item.appendChild(linkTo(reference))
        list.appendChild(item)
      }
      refField.appendChild(list)
      panel.appendChild(refField)
    }

    return panel
  }

  function requestsPanel(issue) {
    const panel = el('div')

    if (issue.request) {
      const wrapper = el('div', 'field')
      wrapper.appendChild(el('p', 'field-label', 'Request'))
      wrapper.appendChild(
        codeBlock('HTTP Request', decodeBase64(issue.request), { truncated: issue.request_truncated }),
      )
      panel.appendChild(wrapper)
    }

    if (issue.response) {
      const wrapper = el('div', 'field')
      wrapper.appendChild(el('p', 'field-label', 'Response'))
      wrapper.appendChild(
        codeBlock('HTTP Response', decodeBase64(issue.response), { truncated: issue.response_truncated }),
      )
      panel.appendChild(wrapper)
    }

    if (!issue.request && !issue.response) {
      panel.appendChild(el('p', 'empty', 'No request or response was captured for this finding'))
    }

    return panel
  }

  function interactionsPanel(issue) {
    const panel = el('div')

    for (const interaction of issue.interactions || []) {
      const card = el('div', 'card')
      card.style.marginBottom = '0.75rem'

      const head = el('div', 'card-head')
      const left = el('div')
      left.style.display = 'flex'
      left.style.alignItems = 'center'
      left.style.gap = '0.5rem'
      left.appendChild(el('span', 'badge badge-outline', (interaction.protocol || 'unknown').toUpperCase()))
      left.appendChild(el('span', null, interaction.remote_address || 'unknown source'))
      head.appendChild(left)
      head.appendChild(el('span', 'stat-hint', interaction.timestamp || ''))
      card.appendChild(head)

      const body = el('div', 'card-body')

      if (interaction.cause) {
        const cause = el('div', 'field')
        cause.appendChild(el('p', 'field-label', 'Triggered by'))
        const grid = el('div')
        grid.style.display = 'grid'
        grid.style.gap = '0.375rem'
        for (const [label, value] of [
          ['Test', interaction.cause.test_name],
          ['Code', interaction.cause.code],
          ['Insertion point', interaction.cause.insertion_point],
          ['Interaction domain', interaction.cause.interaction_domain],
        ]) {
          if (!value) continue
          grid.appendChild(labelledRow(label, value, '9rem'))
        }
        cause.appendChild(grid)

        if (interaction.cause.payload) {
          const payload = el('div')
          payload.style.marginTop = '0.5rem'
          payload.appendChild(codeBlock('Payload', interaction.cause.payload))
          cause.appendChild(payload)
        }
        body.appendChild(cause)
      }

      const protocol = (interaction.protocol || '').toUpperCase()

      const request = decodeBase64(interaction.raw_request)
      if (request) body.appendChild(codeBlock(`${protocol} Request`, request))

      const response = decodeBase64(interaction.raw_response)
      if (response) {
        const spacer = el('div')
        spacer.style.marginTop = '0.5rem'
        spacer.appendChild(codeBlock(`${protocol} Response`, response))
        body.appendChild(spacer)
      }

      card.appendChild(body)
      panel.appendChild(card)
    }

    return panel
  }

  // Some capture sources already prefix the numeric code into the status text
  // ("101 Switching Protocols"), so joining both unconditionally repeats it.
  function formatStatus(code, text) {
    const codeText = code ? String(code) : ''
    const statusText = (text || '').trim()
    if (!codeText) return statusText
    if (!statusText) return codeText
    return statusText.startsWith(codeText) ? statusText : `${codeText} ${statusText}`
  }

  function websocketPanel(issue) {
    const connection = issue.websocket_connection
    const panel = el('div')

    const info = el('div', 'field')
    info.appendChild(el('p', 'field-label', 'Connection'))

    const grid = el('div')
    grid.style.display = 'grid'
    grid.style.gap = '0.375rem'

    // status_code / status_text are the fields the payload actually carries.
    // The previous template read a `status` field that never existed and
    // rendered "undefined" on every WebSocket finding.
    const status = formatStatus(connection.status_code, connection.status_text)

    for (const [label, value] of [
      ['URL', connection.url],
      ['Status', status],
      ['Source', connection.source],
      ['Opened', connection.created_at],
      ['Closed', connection.closed_at],
    ]) {
      if (!value) continue
      const row = labelledRow(label, value, '5rem')
      row.lastChild.style.fontFamily = 'var(--font-mono)'
      row.lastChild.style.fontSize = '0.75rem'
      grid.appendChild(row)
    }

    info.appendChild(grid)
    panel.appendChild(info)

    const messages = connection.messages || []
    const messagesField = el('div', 'field')
    messagesField.appendChild(el('p', 'field-label', `Messages (${messages.length})`))

    if (messages.length === 0) {
      messagesField.appendChild(el('p', 'empty', 'No messages were recorded on this connection'))
    } else {
      for (const message of messages) {
        const row = el('div', `msg msg-${message.direction === 'sent' ? 'sent' : 'received'}`)
        row.appendChild(el('span', 'msg-dir', message.direction === 'sent' ? 'sent →' : '← received'))

        const content = el('div')
        content.style.flex = '1'
        content.style.minWidth = '0'

        let text = message.payload_data || ''
        if (!message.is_binary) {
          try {
            const trimmed = text.trim()
            if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
              text = JSON.stringify(JSON.parse(trimmed), null, 2)
            }
          } catch {
            // Not JSON. The raw payload is what matters.
          }
        }

        content.appendChild(codeBlock(message.timestamp || 'message', text))
        row.appendChild(content)
        messagesField.appendChild(row)
      }
    }

    panel.appendChild(messagesField)
    return panel
  }

  function pocPanel(issue) {
    const panel = el('div')
    if (issue.poc_type) {
      const type = el('div')
      type.style.marginBottom = '0.5rem'
      type.appendChild(el('span', 'badge badge-outline', issue.poc_type))
      panel.appendChild(type)
    }
    panel.appendChild(codeBlock('Proof of Concept', issue.poc || ''))
    return panel
  }

  function buildDetailBody(issue) {
    const definitions = [{ label: 'Description', build: () => descriptionPanel(issue) }]

    if (issue.request || issue.response) {
      definitions.push({ label: 'Request / Response', build: () => requestsPanel(issue) })
    }
    if (issue.interactions && issue.interactions.length > 0) {
      definitions.push({
        label: `Interactions (${issue.interactions.length})`,
        build: () => interactionsPanel(issue),
      })
    }
    if (issue.websocket_connection) {
      definitions.push({ label: 'WebSocket', build: () => websocketPanel(issue) })
    }
    if (issue.poc) {
      definitions.push({ label: 'PoC', build: () => pocPanel(issue) })
    }

    return buildTabs(definitions)
  }

  function bootstrap() {
    bindTheme()
    bindControls()
    window.addEventListener('beforeprint', () => {
      for (const build of panelBuilders) build()
    })
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
