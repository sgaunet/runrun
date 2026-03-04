// WebSocket connection helper
class LogStream {
    constructor(executionID) {
        this.executionID = executionID;
        this.ws = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.onLog = null;
        this.onStatus = null;
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/logs/ws/${this.executionID}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('WebSocket connected for execution:', this.executionID);
            this.reconnectAttempts = 0;
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleMessage(message);
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };

        this.ws.onclose = () => {
            console.log('WebSocket closed');
            this.reconnect();
        };
    }

    handleMessage(message) {
        switch (message.type) {
            case 'log':
                if (this.onLog) {
                    this.onLog(message.data);
                }
                break;
            case 'log_batch':
                if (this.onLog && Array.isArray(message.data)) {
                    for (const entry of message.data) {
                        this.onLog(entry);
                    }
                }
                break;
            case 'subscribed':
                console.log('Subscribed to execution:', message.execution_id);
                if (this.onStatus) {
                    this.onStatus('connected');
                }
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
            console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
            setTimeout(() => this.connect(), delay);
        } else {
            console.log('Max reconnect attempts reached, falling back to polling');
            if (this.onStatus) {
                this.onStatus('fallback');
            }
        }
    }

    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }
}

// Polling fallback helper
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
        fetch(`/api/logs/${this.executionID}/poll?lines=100`)
            .then(res => res.json())
            .then(data => {
                if (data.logs && data.logs.length > this.lastLogCount) {
                    // New logs available
                    const newLogs = data.logs.slice(this.lastLogCount);
                    this.lastLogCount = data.logs.length;

                    if (this.onLog) {
                        newLogs.forEach(line => {
                            this.onLog({ line: line, timestamp: new Date().toISOString() });
                        });
                    }
                }
            })
            .catch(err => console.error('Polling error:', err));
    }

    stop() {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = null;
        }
    }
}
