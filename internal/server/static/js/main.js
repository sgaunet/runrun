// RunRun page-level glue.
//
// Under strict CSP (no 'unsafe-inline', no 'unsafe-eval') all <script>
// tags carry a per-request nonce supplied by CSPNonceMiddleware. This
// file is loaded once per page; it exposes the public surface used by
// templ pages on window.RunRun, and wires delegated listeners for the
// data-action attributes that are global to all pages (currently just
// the mobile navbar burger toggle).
//
// Per-page wiring (dashboard filters, task-detail run-task confirms,
// logs page log-viewer) lives in the templ `script` blocks of each
// page, where they get rendered under the same nonce.

(function () {
  'use strict';

  // -----------------------------------------------------------------
  // WebSocket connection helper. Streams logs from a single execution
  // and falls back to polling after maxReconnectAttempts.
  // -----------------------------------------------------------------
  class LogStream {
    constructor(executionID) {
      this.executionID = executionID;
      this.ws = null;
      this.reconnectAttempts = 0;
      this.maxReconnectAttempts = 5;
      this.onLog = null;
      this.onStatus = null;
      this.onMetadata = null;
    }

    connect() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/logs/ws/${this.executionID}`;
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.reconnectAttempts = 0;
      };

      this.ws.onmessage = (event) => {
        try {
          this.handleMessage(JSON.parse(event.data));
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e);
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
      };

      this.ws.onclose = () => {
        this.reconnect();
      };
    }

    handleMessage(message) {
      switch (message.type) {
        case 'log':
          if (this.onLog) this.onLog(message.data);
          break;
        case 'log_batch':
          if (this.onLog && Array.isArray(message.data)) {
            for (const entry of message.data) this.onLog(entry);
          }
          break;
        case 'metadata':
          if (this.onMetadata) this.onMetadata(message.data);
          break;
        case 'subscribed':
          if (this.onStatus) this.onStatus('connected');
          break;
        case 'error':
          console.error('WebSocket error message:', message.error);
          break;
      }
    }

    reconnect() {
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
        setTimeout(() => this.connect(), delay);
      } else if (this.onStatus) {
        this.onStatus('fallback');
      }
    }

    disconnect() {
      if (this.ws) {
        this.ws.close();
        this.ws = null;
      }
    }
  }

  // -----------------------------------------------------------------
  // Polling fallback for the logs page when WS reconnects are
  // exhausted.
  // -----------------------------------------------------------------
  class LogPoller {
    constructor(executionID, interval = 3000) {
      this.executionID = executionID;
      this.interval = interval;
      this.pollTimer = null;
      this.lastLogCount = 0;
      this.onLog = null;
    }

    start() {
      this.poll();
      this.pollTimer = setInterval(() => this.poll(), this.interval);
    }

    poll() {
      fetch(`/logs/${this.executionID}/poll?lines=100`, { credentials: 'same-origin' })
        .then((res) => res.json())
        .then((data) => {
          if (data.logs && data.logs.length > this.lastLogCount) {
            const newLogs = data.logs.slice(this.lastLogCount);
            this.lastLogCount = data.logs.length;
            if (this.onLog) {
              newLogs.forEach((line) => {
                this.onLog({ line: line, timestamp: new Date().toISOString() });
              });
            }
          }
        })
        .catch((err) => console.error('Polling error:', err));
    }

    stop() {
      if (this.pollTimer) {
        clearInterval(this.pollTimer);
        this.pollTimer = null;
      }
    }
  }

  // -----------------------------------------------------------------
  // CSRF helper. Reads the CSRF token from the <meta name="csrf-token">
  // tag emitted by base.templ; used by data-action handlers that POST.
  // -----------------------------------------------------------------
  function csrfToken() {
    const meta = document.querySelector('meta[name="csrf-token"]');
    return meta ? meta.getAttribute('content') || '' : '';
  }

  // -----------------------------------------------------------------
  // Delegated data-action handlers. New per-page actions plug in here
  // by adding a case to the dispatcher.
  // -----------------------------------------------------------------
  function onClickDispatch(event) {
    const el = event.target.closest('[data-action]');
    if (!el) return;
    const action = el.getAttribute('data-action');
    switch (action) {
      case 'toggle-burger': {
        // Bulma navbar burger -> menu open/close
        event.preventDefault();
        el.classList.toggle('is-active');
        const targetSel = el.getAttribute('data-target');
        if (targetSel) {
          const menu = document.getElementById(targetSel);
          if (menu) menu.classList.toggle('is-active');
        }
        return;
      }
      case 'run-task': {
        event.preventDefault();
        const taskName = el.getAttribute('data-task-name');
        if (!taskName) return;
        runTask(taskName, el);
        return;
      }
      case 'clear-filters': {
        event.preventDefault();
        clearFilters();
        return;
      }
      default:
        // Unknown action — let the event continue.
        return;
    }
  }

  function runTask(taskName, button) {
    const originalText = button.textContent;
    button.disabled = true;
    button.classList.add('is-loading');
    fetch('/tasks/' + encodeURIComponent(taskName) + '/execute', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'X-CSRF-Token': csrfToken() },
    })
      .then((res) => res.json())
      .then((data) => {
        if (data.success || data.execution_id || data.status === 'queued') {
          if (data.execution_id) {
            window.location.href = '/logs/' + encodeURIComponent(data.execution_id);
            return;
          }
          window.location.reload();
        } else {
          showInlineError(button, data.message || 'Failed to start task');
        }
      })
      .catch((err) => showInlineError(button, 'Failed to start task: ' + err))
      .finally(() => {
        button.disabled = false;
        button.classList.remove('is-loading');
        button.textContent = originalText;
      });
  }

  function showInlineError(anchor, message) {
    // Prefer a per-task error pane keyed by data-task-name; fall back
    // to a Bulma notification injected near the trigger.
    const taskName = anchor.getAttribute('data-task-name');
    if (taskName) {
      const errDiv = document.getElementById('error-' + taskName);
      if (errDiv) {
        errDiv.textContent = message;
        errDiv.classList.remove('is-hidden');
        setTimeout(() => errDiv.classList.add('is-hidden'), 5000);
        return;
      }
    }
    console.error(message);
  }

  function clearFilters() {
    const search = document.getElementById('searchInput');
    const status = document.getElementById('statusFilter');
    const tag = document.getElementById('tagFilter');
    if (search) search.value = '';
    if (status) status.value = 'all';
    if (tag) tag.value = 'all';
    if (typeof window.filterTasks === 'function') window.filterTasks();
  }

  // -----------------------------------------------------------------
  // Public surface (read by templ-rendered <script> blocks under nonce).
  // -----------------------------------------------------------------
  window.RunRun = window.RunRun || {};
  window.RunRun.LogStream = LogStream;
  window.RunRun.LogPoller = LogPoller;
  window.RunRun.csrfToken = csrfToken;

  document.addEventListener('DOMContentLoaded', () => {
    document.addEventListener('click', onClickDispatch);
  });
})();
