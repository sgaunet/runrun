// LogViewer - Advanced log viewer with ANSI color support and auto-scroll
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
            ...options
        };

        this.lines = [];
        this.filteredLines = [];
        this.currentFilter = 'all';
        this.searchTerm = '';
        this.autoScrollEnabled = this.options.autoScroll;
        this.userHasScrolled = false;
        this.totalLines = null; // Total lines from server metadata (null = unknown)
        this.ansiUp = new AnsiUp();
        this.ansiUp.use_classes = true;

        this.init();
    }

    init() {
        this.container.innerHTML = `
            <div class="log-viewer-wrapper">
                <div class="log-viewer-controls bg-white border-b border-gray-200 p-4 flex flex-wrap gap-4 items-center">
                    <div class="flex gap-2">
                        <button id="clearLogsBtn" class="btn btn-secondary text-sm">Clear</button>
                        <button id="downloadLogsBtn" class="btn btn-secondary text-sm">Download</button>
                        <button id="copyLogsBtn" class="btn btn-secondary text-sm">Copy</button>
                    </div>
                    <div class="flex gap-2 items-center">
                        <label class="text-sm text-gray-600">Filter:</label>
                        <select id="logLevelFilter" class="form-input text-sm py-1">
                            <option value="all">All</option>
                            <option value="debug">Debug</option>
                            <option value="info">Info</option>
                            <option value="warn">Warn</option>
                            <option value="error">Error</option>
                        </select>
                    </div>
                    <div class="flex-1">
                        <input type="text" id="logSearch" placeholder="Search logs..." class="form-input text-sm w-full max-w-md" />
                    </div>
                    <div class="flex items-center gap-2 text-sm text-gray-600">
                        <span>Lines: <span id="lineCount">0</span></span>
                    </div>
                    <div class="flex items-center gap-2">
                        <input type="checkbox" id="autoScrollToggle" checked class="rounded" />
                        <label for="autoScrollToggle" class="text-sm text-gray-600">Auto-scroll</label>
                    </div>
                </div>
                <div class="log-viewer-content relative">
                    <div id="logLines" class="log-container scrollbar-thin" style="max-height: 600px; overflow-y: auto;">
                        <div class="text-gray-500 p-4">Waiting for logs...</div>
                    </div>
                    <button id="scrollToBottomBtn" class="hidden fixed bottom-20 right-8 bg-primary-600 text-white px-4 py-2 rounded-full shadow-lg hover:bg-primary-700 transition-colors">
                        ↓ Scroll to Bottom
                    </button>
                </div>
            </div>
        `;

        this.logLinesContainer = document.getElementById('logLines');
        this.scrollToBottomBtn = document.getElementById('scrollToBottomBtn');
        this.autoScrollToggle = document.getElementById('autoScrollToggle');

        this.setupEventListeners();
    }

    setupEventListeners() {
        // Auto-scroll toggle
        this.autoScrollToggle.addEventListener('change', (e) => {
            this.autoScrollEnabled = e.target.checked;
            if (this.autoScrollEnabled) {
                this.scrollToBottom();
            }
        });

        // Scroll event detection
        this.logLinesContainer.addEventListener('scroll', () => {
            const isAtBottom = this.isScrolledToBottom();

            if (!isAtBottom && this.autoScrollEnabled) {
                this.userHasScrolled = true;
                this.showScrollToBottomButton();
            } else if (isAtBottom) {
                this.userHasScrolled = false;
                this.hideScrollToBottomButton();
            }
        });

        // Scroll to bottom button
        this.scrollToBottomBtn.addEventListener('click', () => {
            this.scrollToBottom();
            this.autoScrollEnabled = true;
            this.autoScrollToggle.checked = true;
            this.userHasScrolled = false;
            this.hideScrollToBottomButton();
        });

        // Clear logs
        document.getElementById('clearLogsBtn').addEventListener('click', () => {
            this.clear();
        });

        // Download logs
        document.getElementById('downloadLogsBtn').addEventListener('click', () => {
            this.download();
        });

        // Copy logs
        document.getElementById('copyLogsBtn').addEventListener('click', () => {
            this.copy();
        });

        // Log level filter
        document.getElementById('logLevelFilter').addEventListener('change', (e) => {
            this.currentFilter = e.target.value;
            this.applyFilters();
        });

        // Search
        const searchInput = document.getElementById('logSearch');
        let searchTimeout;
        searchInput.addEventListener('input', (e) => {
            clearTimeout(searchTimeout);
            searchTimeout = setTimeout(() => {
                this.searchTerm = e.target.value.toLowerCase();
                this.applyFilters();
            }, 300);
        });
    }

    isScrolledToBottom() {
        const threshold = 50;
        const position = this.logLinesContainer.scrollTop + this.logLinesContainer.clientHeight;
        const height = this.logLinesContainer.scrollHeight;
        return position >= height - threshold;
    }

    scrollToBottom() {
        this.logLinesContainer.scrollTop = this.logLinesContainer.scrollHeight;
    }

    showScrollToBottomButton() {
        this.scrollToBottomBtn.classList.remove('hidden');
    }

    hideScrollToBottomButton() {
        this.scrollToBottomBtn.classList.add('hidden');
    }

    addLine(text, metadata = {}) {
        const lineData = {
            text: text,
            timestamp: metadata.timestamp || new Date(),
            level: metadata.level || this.detectLogLevel(text),
            raw: text
        };

        this.lines.push(lineData);

        // Keep only max lines
        if (this.lines.length > this.options.maxLines) {
            this.lines.shift();
        }

        this.applyFilters();

        // Auto-scroll if enabled and user hasn't manually scrolled
        if (this.autoScrollEnabled && !this.userHasScrolled) {
            this.scrollToBottom();
        }
    }

    detectLogLevel(text) {
        const lowerText = text.toLowerCase();
        if (lowerText.includes('error') || lowerText.includes('fatal')) return 'error';
        if (lowerText.includes('warn')) return 'warn';
        if (lowerText.includes('info')) return 'info';
        if (lowerText.includes('debug')) return 'debug';
        return 'info';
    }

    applyFilters() {
        this.filteredLines = this.lines.filter(line => {
            // Apply level filter
            if (this.currentFilter !== 'all' && line.level !== this.currentFilter) {
                return false;
            }

            // Apply search filter
            if (this.searchTerm && !line.text.toLowerCase().includes(this.searchTerm)) {
                return false;
            }

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
        const el = document.getElementById('lineCount');
        if (!el) return;

        const received = this.lines.length;
        const shown = this.filteredLines.length;
        const total = this.totalLines;

        if (total !== null && shown !== received) {
            // Filtered view with known total: "shown / received of total"
            el.textContent = `${shown} / ${received} of ${total}`;
        } else if (total !== null) {
            // Known total: "received / total"
            el.textContent = `${received} / ${total}`;
        } else if (shown !== received) {
            // Filtered view, unknown total: "shown / received"
            el.textContent = `${shown} / ${received}`;
        } else {
            el.textContent = `${received}`;
        }
    }

    getLevelClass(level) {
        const classes = {
            error: 'border-l-2 border-danger-500 bg-danger-900 bg-opacity-10',
            warn: 'border-l-2 border-warning-500 bg-warning-900 bg-opacity-10',
            info: '',
            debug: 'text-gray-400'
        };
        return classes[level] || '';
    }

    formatTimestamp(date) {
        const pad = (n) => n.toString().padStart(2, '0');
        return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
    }

    clear() {
        this.lines = [];
        this.filteredLines = [];
        this.render();
        this.updateLineCount();
    }

    download() {
        const text = this.lines.map(line => {
            const timestamp = this.formatTimestamp(line.timestamp);
            return `[${timestamp}] ${line.text}`;
        }).join('\n');

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
        const text = this.lines.map(line => line.text).join('\n');
        const btn = document.getElementById('copyLogsBtn');
        navigator.clipboard.writeText(text).then(() => {
            if (btn) {
                const orig = btn.textContent;
                btn.textContent = 'Copied!';
                setTimeout(() => { btn.textContent = orig; }, 2000);
            }
        }).catch(err => {
            console.error('Failed to copy logs:', err);
        });
    }

    getAllLinesAsText() {
        return this.lines.map(line => line.text).join('\n');
    }

    // Batch add lines for better performance
    addLines(lines) {
        lines.forEach(line => {
            const lineData = typeof line === 'string'
                ? { text: line, timestamp: new Date(), level: this.detectLogLevel(line), raw: line }
                : { text: line.text, timestamp: line.timestamp || new Date(), level: line.level || this.detectLogLevel(line.text), raw: line.text };

            this.lines.push(lineData);
        });

        // Trim to max lines
        if (this.lines.length > this.options.maxLines) {
            this.lines = this.lines.slice(-this.options.maxLines);
        }

        this.applyFilters();

        if (this.autoScrollEnabled && !this.userHasScrolled) {
            this.scrollToBottom();
        }
    }

    // === Pagination Mode ===

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
        const wrapper = this.container.querySelector('.log-viewer-wrapper');
        if (!wrapper) return;

        // Insert pagination bar after controls
        const controls = wrapper.querySelector('.log-viewer-controls');
        if (!controls) return;

        // Remove existing pagination bar if any
        const existing = wrapper.querySelector('.log-pagination-controls');
        if (existing) existing.remove();

        const bar = document.createElement('div');
        bar.className = 'log-pagination-controls bg-gray-50 border-b border-gray-200 px-4 py-2 flex flex-wrap gap-3 items-center text-sm';
        bar.innerHTML = `
            <div class="flex gap-1 items-center">
                <button id="pgFirst" class="btn btn-secondary text-xs px-2 py-1" title="Go to Start">&#171; First</button>
                <button id="pgPrev" class="btn btn-secondary text-xs px-2 py-1" title="Previous Page">&#8249; Prev</button>
                <button id="pgNext" class="btn btn-secondary text-xs px-2 py-1" title="Next Page">Next &#8250;</button>
                <button id="pgLast" class="btn btn-secondary text-xs px-2 py-1" title="Go to End">Last &#187;</button>
            </div>
            <div class="flex gap-2 items-center">
                <label class="text-gray-600">Page size:</label>
                <select id="pgPageSize" class="form-input text-xs py-1 w-20">
                    <option value="100">100</option>
                    <option value="500" selected>500</option>
                    <option value="1000">1000</option>
                </select>
            </div>
            <div class="flex gap-2 items-center">
                <label class="text-gray-600">Jump to line:</label>
                <input type="number" id="pgJumpLine" class="form-input text-xs py-1 w-24" min="0" placeholder="0" />
                <button id="pgJumpBtn" class="btn btn-secondary text-xs px-2 py-1">Go</button>
            </div>
            <div class="flex-1 text-right text-gray-600">
                <span id="pgIndicator">Loading...</span>
            </div>
        `;

        controls.after(bar);

        // Event listeners
        document.getElementById('pgFirst').addEventListener('click', () => this.loadPage(0, this.pageSize));
        document.getElementById('pgPrev').addEventListener('click', () => {
            const newStart = Math.max(0, this.pageStart - this.pageSize);
            this.loadPage(newStart, this.pageSize);
        });
        document.getElementById('pgNext').addEventListener('click', () => {
            const newStart = this.pageStart + this.pageSize;
            if (newStart < this.pageTotalLines) {
                this.loadPage(newStart, this.pageSize);
            }
        });
        document.getElementById('pgLast').addEventListener('click', () => {
            const newStart = Math.max(0, this.pageTotalLines - this.pageSize);
            this.loadPage(newStart, this.pageSize);
        });
        document.getElementById('pgPageSize').addEventListener('change', (e) => {
            this.pageSize = parseInt(e.target.value, 10);
            this.loadPage(this.pageStart, this.pageSize);
        });
        document.getElementById('pgJumpBtn').addEventListener('click', () => {
            const line = parseInt(document.getElementById('pgJumpLine').value, 10);
            if (!isNaN(line) && line >= 0) {
                this.jumpToLine(line);
            }
        });
        document.getElementById('pgJumpLine').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                document.getElementById('pgJumpBtn').click();
            }
        });
    }

    updatePaginationIndicator(start, count, total) {
        const el = document.getElementById('pgIndicator');
        if (!el) return;
        const end = Math.min(start + count, total);
        el.textContent = `Lines ${start + 1}–${end} of ${total.toLocaleString()}`;
    }

    loadPage(start, count) {
        const url = `/logs/${this.paginationExecId}/segment?start=${start}&count=${count}`;
        fetch(url)
            .then(res => {
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                return res.json();
            })
            .then(data => {
                this.pageStart = data.start;
                this.pageTotalLines = data.total_lines;

                // Clear and load new lines
                this.lines = [];
                this.filteredLines = [];
                if (data.lines && data.lines.length > 0) {
                    for (const entry of data.lines) {
                        this.lines.push({
                            text: entry.line,
                            timestamp: new Date(),
                            level: entry.level || 'info',
                            raw: entry.line,
                            lineNumber: entry.number
                        });
                    }
                }
                this.applyFilters();
                this.setTotalLines(data.total_lines);
                this.updatePaginationIndicator(data.start, data.lines ? data.lines.length : 0, data.total_lines);

                // Scroll to top of log content
                if (this.logLinesContainer) {
                    this.logLinesContainer.scrollTop = 0;
                }

                // Update button states
                this.updatePaginationButtons();
            })
            .catch(err => {
                console.error('Failed to load log segment:', err);
            });
    }

    jumpToLine(lineNumber) {
        // Load a page centered around the target line
        const start = Math.max(0, lineNumber - Math.floor(this.pageSize / 4));
        this.loadPage(start, this.pageSize);
    }

    updatePaginationButtons() {
        const first = document.getElementById('pgFirst');
        const prev = document.getElementById('pgPrev');
        const next = document.getElementById('pgNext');
        const last = document.getElementById('pgLast');
        if (!first) return;

        const atStart = this.pageStart <= 0;
        const atEnd = this.pageStart + this.pageSize >= this.pageTotalLines;

        first.disabled = atStart;
        prev.disabled = atStart;
        next.disabled = atEnd;
        last.disabled = atEnd;
    }

    // Override render in pagination mode to use line numbers from server
    render() {
        if (this.filteredLines.length === 0) {
            this.logLinesContainer.innerHTML = '<div class="text-gray-500 p-4">No logs to display</div>';
            return;
        }

        const html = this.filteredLines.map((line, index) => {
            const ansiHtml = this.ansiUp.ansi_to_html(line.text);
            const levelClass = this.getLevelClass(line.level);
            const timestamp = this.options.showTimestamps
                ? `<span class="text-gray-500 text-xs mr-2">${this.formatTimestamp(line.timestamp)}</span>`
                : '';
            const displayNum = line.lineNumber != null ? line.lineNumber + 1 : index + 1;
            const lineNumber = this.options.showLineNumbers
                ? `<span class="text-gray-400 text-xs mr-2 select-none">${displayNum.toString().padStart(4, ' ')}</span>`
                : '';

            return `<div class="log-line ${levelClass} py-1 px-2 hover:bg-gray-800">${lineNumber}${timestamp}<span>${ansiHtml}</span></div>`;
        }).join('');

        this.logLinesContainer.innerHTML = html;
    }
}
