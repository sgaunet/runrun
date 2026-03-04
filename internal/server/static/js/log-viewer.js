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

    updateLineCount() {
        const el = document.getElementById('lineCount');
        if (!el) return;
        if (this.filteredLines.length !== this.lines.length) {
            el.textContent = `${this.filteredLines.length} / ${this.lines.length}`;
        } else {
            el.textContent = `${this.lines.length}`;
        }
    }

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
            const lineNumber = this.options.showLineNumbers
                ? `<span class="text-gray-400 text-xs mr-2 select-none">${(index + 1).toString().padStart(4, ' ')}</span>`
                : '';

            return `<div class="log-line ${levelClass} py-1 px-2 hover:bg-gray-800">${lineNumber}${timestamp}<span>${ansiHtml}</span></div>`;
        }).join('');

        this.logLinesContainer.innerHTML = html;
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
            if (typeof line === 'string') {
                this.addLine(line);
            } else {
                this.addLine(line.text, line);
            }
        });
    }
}
