// RunRun LogViewer — Bulma-styled log viewer with ANSI rendering,
// search, level filter, auto-scroll, and segment pagination.
//
// Loaded by base.templ on every page; instantiated only on the logs
// page via:
//   const viewer = new window.RunRun.LogViewer('logViewer', { ... });
//
// CSP-safe: every event handler is attached with addEventListener;
// no inline on* attributes are emitted.

(function () {
  'use strict';

  class LogViewer {
    constructor(containerId, options = {}) {
      this.container = document.getElementById(containerId);
      if (!this.container) {
        throw new Error(`Container element with id "${containerId}" not found`);
      }

      this.options = {
        autoScroll: true,
        showLineNumbers: options.showLineNumbers !== false,
        showTimestamps: options.showTimestamps !== false,
        maxLines: options.maxLines || 10000,
        ...options,
      };

      this.lines = [];
      this.filteredLines = [];
      this.currentFilter = 'all';
      this.searchTerm = '';
      this.autoScrollEnabled = this.options.autoScroll;
      this.userHasScrolled = false;
      this.totalLines = null;
      this.ansiUp = new AnsiUp();
      this.ansiUp.use_classes = true;

      this.init();
    }

    init() {
      this.container.innerHTML = `
        <div class="box runrun-log-viewer">
          <div class="field is-grouped is-grouped-multiline">
            <p class="control">
              <button class="button is-small" data-role="clear">Clear</button>
            </p>
            <p class="control">
              <button class="button is-small" data-role="download">Download</button>
            </p>
            <p class="control">
              <button class="button is-small" data-role="copy">Copy</button>
            </p>
            <p class="control">
              <span class="select is-small">
                <select data-role="levelFilter">
                  <option value="all">All</option>
                  <option value="debug">Debug</option>
                  <option value="info">Info</option>
                  <option value="warn">Warn</option>
                  <option value="error">Error</option>
                </select>
              </span>
            </p>
            <p class="control is-expanded">
              <input class="input is-small" type="text" data-role="search" placeholder="Search logs..." />
            </p>
            <p class="control">
              <span class="tag is-info is-light">Lines: <span data-role="lineCount">0</span></span>
            </p>
            <p class="control">
              <label class="checkbox is-size-7">
                <input type="checkbox" data-role="autoScroll" checked /> Auto-scroll
              </label>
            </p>
          </div>
          <div data-role="logLines" class="runrun-log-container">
            <div class="has-text-grey">Waiting for logs...</div>
          </div>
          <button class="button is-primary is-rounded is-hidden runrun-scroll-to-bottom"
                  data-role="scrollToBottom"
                  type="button">
            ↓ Scroll to Bottom
          </button>
        </div>
      `;

      this.logLinesContainer = this.container.querySelector('[data-role="logLines"]');
      this.scrollToBottomBtn = this.container.querySelector('[data-role="scrollToBottom"]');
      this.autoScrollToggle = this.container.querySelector('[data-role="autoScroll"]');

      this.setupEventListeners();
    }

    setupEventListeners() {
      this.autoScrollToggle.addEventListener('change', (e) => {
        this.autoScrollEnabled = e.target.checked;
        if (this.autoScrollEnabled) this.scrollToBottom();
      });

      this.logLinesContainer.addEventListener('scroll', () => {
        const isAtBottom = this.isScrolledToBottom();
        if (!isAtBottom && this.autoScrollEnabled) {
          this.userHasScrolled = true;
          this.scrollToBottomBtn.classList.remove('is-hidden');
        } else if (isAtBottom) {
          this.userHasScrolled = false;
          this.scrollToBottomBtn.classList.add('is-hidden');
        }
      });

      this.scrollToBottomBtn.addEventListener('click', () => {
        this.scrollToBottom();
        this.autoScrollEnabled = true;
        this.autoScrollToggle.checked = true;
        this.userHasScrolled = false;
        this.scrollToBottomBtn.classList.add('is-hidden');
      });

      this.container.querySelector('[data-role="clear"]').addEventListener('click', () => this.clear());
      this.container.querySelector('[data-role="download"]').addEventListener('click', () => this.download());
      this.container.querySelector('[data-role="copy"]').addEventListener('click', () => this.copy());

      this.container.querySelector('[data-role="levelFilter"]').addEventListener('change', (e) => {
        this.currentFilter = e.target.value;
        this.applyFilters();
      });

      const search = this.container.querySelector('[data-role="search"]');
      let searchTimer;
      search.addEventListener('input', (e) => {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
          this.searchTerm = e.target.value.toLowerCase();
          this.applyFilters();
        }, 300);
      });
    }

    isScrolledToBottom() {
      const threshold = 50;
      const pos = this.logLinesContainer.scrollTop + this.logLinesContainer.clientHeight;
      return pos >= this.logLinesContainer.scrollHeight - threshold;
    }

    scrollToBottom() {
      this.logLinesContainer.scrollTop = this.logLinesContainer.scrollHeight;
    }

    addLine(text, metadata = {}) {
      const lineData = {
        text: text,
        timestamp: metadata.timestamp || new Date(),
        level: metadata.level || this.detectLogLevel(text),
        raw: text,
      };
      this.lines.push(lineData);
      if (this.lines.length > this.options.maxLines) this.lines.shift();
      this.applyFilters();
      if (this.autoScrollEnabled && !this.userHasScrolled) this.scrollToBottom();
    }

    addLines(lines) {
      lines.forEach((line) => {
        const lineData =
          typeof line === 'string'
            ? { text: line, timestamp: new Date(), level: this.detectLogLevel(line), raw: line }
            : {
                text: line.text,
                timestamp: line.timestamp || new Date(),
                level: line.level || this.detectLogLevel(line.text),
                raw: line.text,
              };
        this.lines.push(lineData);
      });
      if (this.lines.length > this.options.maxLines) {
        this.lines = this.lines.slice(-this.options.maxLines);
      }
      this.applyFilters();
      if (this.autoScrollEnabled && !this.userHasScrolled) this.scrollToBottom();
    }

    detectLogLevel(text) {
      const lower = text.toLowerCase();
      if (lower.includes('error') || lower.includes('fatal')) return 'error';
      if (lower.includes('warn')) return 'warn';
      if (lower.includes('info')) return 'info';
      if (lower.includes('debug')) return 'debug';
      return 'info';
    }

    applyFilters() {
      this.filteredLines = this.lines.filter((line) => {
        if (this.currentFilter !== 'all' && line.level !== this.currentFilter) return false;
        if (this.searchTerm && !line.text.toLowerCase().includes(this.searchTerm)) return false;
        return true;
      });
      this.render();
      this.updateLineCount();
    }

    setTotalLines(total) {
      this.totalLines = total;
      this.updateLineCount();
    }

    updateLineCount() {
      const el = this.container.querySelector('[data-role="lineCount"]');
      if (!el) return;
      const received = this.lines.length;
      const shown = this.filteredLines.length;
      const total = this.totalLines;
      if (total !== null && shown !== received) {
        el.textContent = `${shown} / ${received} of ${total}`;
      } else if (total !== null) {
        el.textContent = `${received} / ${total}`;
      } else if (shown !== received) {
        el.textContent = `${shown} / ${received}`;
      } else {
        el.textContent = `${received}`;
      }
    }

    levelClass(level) {
      switch (level) {
        case 'error': return 'runrun-log-line runrun-log-line--error';
        case 'warn':  return 'runrun-log-line runrun-log-line--warn';
        case 'debug': return 'runrun-log-line runrun-log-line--debug';
        default:      return 'runrun-log-line runrun-log-line--info';
      }
    }

    formatTimestamp(date) {
      const pad = (n) => n.toString().padStart(2, '0');
      return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
    }

    render() {
      if (this.filteredLines.length === 0) {
        this.logLinesContainer.innerHTML =
          '<div class="has-text-grey is-italic">No logs to display</div>';
        return;
      }
      const html = this.filteredLines.map((line, index) => {
        const ansiHtml = this.ansiUp.ansi_to_html(line.text);
        const cls = this.levelClass(line.level);
        const tsPrefix = this.options.showTimestamps
          ? `<span class="has-text-grey is-size-7 mr-2">${this.formatTimestamp(line.timestamp)}</span>`
          : '';
        const displayNum = line.lineNumber != null ? line.lineNumber + 1 : index + 1;
        const numPrefix = this.options.showLineNumbers
          ? `<span class="has-text-grey-light is-size-7 mr-2">${displayNum.toString().padStart(4, ' ')}</span>`
          : '';
        return `<span class="${cls}">${numPrefix}${tsPrefix}<span>${ansiHtml}</span></span>`;
      }).join('');
      this.logLinesContainer.innerHTML = html;
    }

    clear() {
      this.lines = [];
      this.filteredLines = [];
      this.render();
      this.updateLineCount();
    }

    download() {
      const text = this.lines
        .map((l) => `[${this.formatTimestamp(l.timestamp)}] ${l.text}`)
        .join('\n');
      const blob = new Blob([text], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `logs-${Date.now()}.txt`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }

    copy() {
      const text = this.lines.map((l) => l.text).join('\n');
      const btn = this.container.querySelector('[data-role="copy"]');
      navigator.clipboard.writeText(text).then(
        () => {
          if (!btn) return;
          const orig = btn.textContent;
          btn.textContent = 'Copied!';
          setTimeout(() => { btn.textContent = orig; }, 2000);
        },
        (err) => console.error('Failed to copy logs:', err),
      );
    }

    getAllLinesAsText() {
      return this.lines.map((l) => l.text).join('\n');
    }

    // -------- Pagination mode --------

    enablePagination(executionId) {
      this.paginationMode = true;
      this.paginationExecId = executionId;
      this.pageSize = 500;
      this.pageStart = 0;
      this.pageTotalLines = 0;
      this.renderPaginationControls();
      this.loadPage(0, this.pageSize);
    }

    renderPaginationControls() {
      const viewer = this.container.querySelector('.runrun-log-viewer');
      if (!viewer) return;
      const existing = viewer.querySelector('[data-role="paginationBar"]');
      if (existing) existing.remove();

      const bar = document.createElement('nav');
      bar.className = 'level is-mobile mt-3 is-size-7';
      bar.setAttribute('data-role', 'paginationBar');
      bar.innerHTML = `
        <div class="level-left">
          <div class="buttons has-addons">
            <button class="button is-small" data-role="pgFirst" title="Go to start">&laquo; First</button>
            <button class="button is-small" data-role="pgPrev"  title="Previous page">&lsaquo; Prev</button>
            <button class="button is-small" data-role="pgNext"  title="Next page">Next &rsaquo;</button>
            <button class="button is-small" data-role="pgLast"  title="Go to end">Last &raquo;</button>
          </div>
        </div>
        <div class="level-item">
          <div class="field is-grouped">
            <p class="control">
              <span class="select is-small">
                <select data-role="pgPageSize">
                  <option value="100">100</option>
                  <option value="500" selected>500</option>
                  <option value="1000">1000</option>
                </select>
              </span>
            </p>
            <p class="control">
              <input class="input is-small runrun-w-jump" type="number" min="0" placeholder="Line #" data-role="pgJumpLine"/>
            </p>
            <p class="control">
              <button class="button is-small" data-role="pgJump">Go</button>
            </p>
          </div>
        </div>
        <div class="level-right">
          <span class="has-text-grey" data-role="pgIndicator">Loading...</span>
        </div>
      `;
      viewer.appendChild(bar);

      const q = (sel) => bar.querySelector(`[data-role="${sel}"]`);
      q('pgFirst').addEventListener('click', () => this.loadPage(0, this.pageSize));
      q('pgPrev').addEventListener('click', () => {
        const newStart = Math.max(0, this.pageStart - this.pageSize);
        this.loadPage(newStart, this.pageSize);
      });
      q('pgNext').addEventListener('click', () => {
        const newStart = this.pageStart + this.pageSize;
        if (newStart < this.pageTotalLines) this.loadPage(newStart, this.pageSize);
      });
      q('pgLast').addEventListener('click', () => {
        const newStart = Math.max(0, this.pageTotalLines - this.pageSize);
        this.loadPage(newStart, this.pageSize);
      });
      q('pgPageSize').addEventListener('change', (e) => {
        this.pageSize = parseInt(e.target.value, 10);
        this.loadPage(this.pageStart, this.pageSize);
      });
      q('pgJump').addEventListener('click', () => {
        const line = parseInt(q('pgJumpLine').value, 10);
        if (!isNaN(line) && line >= 0) this.jumpToLine(line);
      });
      q('pgJumpLine').addEventListener('keydown', (e) => {
        if (e.key === 'Enter') q('pgJump').click();
      });
    }

    updatePaginationIndicator(start, count, total) {
      const el = this.container.querySelector('[data-role="pgIndicator"]');
      if (!el) return;
      const end = Math.min(start + count, total);
      el.textContent = `Lines ${start + 1}–${end} of ${total.toLocaleString()}`;
    }

    loadPage(start, count) {
      const url = `/logs/${this.paginationExecId}/segment?start=${start}&count=${count}`;
      fetch(url, { credentials: 'same-origin' })
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.json();
        })
        .then((data) => {
          this.pageStart = data.start;
          this.pageTotalLines = data.total_lines;
          this.lines = [];
          this.filteredLines = [];
          if (data.lines && data.lines.length > 0) {
            for (const entry of data.lines) {
              this.lines.push({
                text: entry.line,
                timestamp: new Date(),
                level: entry.level || 'info',
                raw: entry.line,
                lineNumber: entry.number,
              });
            }
          }
          this.applyFilters();
          this.setTotalLines(data.total_lines);
          this.updatePaginationIndicator(
            data.start,
            data.lines ? data.lines.length : 0,
            data.total_lines,
          );
          if (this.logLinesContainer) this.logLinesContainer.scrollTop = 0;
          this.updatePaginationButtons();
        })
        .catch((err) => console.error('Failed to load log segment:', err));
    }

    jumpToLine(lineNumber) {
      const start = Math.max(0, lineNumber - Math.floor(this.pageSize / 4));
      this.loadPage(start, this.pageSize);
    }

    updatePaginationButtons() {
      const q = (sel) => this.container.querySelector(`[data-role="${sel}"]`);
      const first = q('pgFirst');
      if (!first) return;
      const atStart = this.pageStart <= 0;
      const atEnd = this.pageStart + this.pageSize >= this.pageTotalLines;
      first.disabled = atStart;
      q('pgPrev').disabled = atStart;
      q('pgNext').disabled = atEnd;
      q('pgLast').disabled = atEnd;
    }
  }

  window.RunRun = window.RunRun || {};
  window.RunRun.LogViewer = LogViewer;
})();
