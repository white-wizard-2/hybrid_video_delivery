// Check if Vue is loaded
if (typeof Vue === 'undefined') {
    console.error('Vue.js is not loaded!');
    document.body.innerHTML = '<div style="padding: 20px; color: red;"><h1>Error: Vue.js failed to load</h1><p>Please check your internet connection and refresh the page.</p></div>';
} else {
    console.log('Vue.js loaded successfully');
}

const { createApp } = Vue;

createApp({
    data() {
        return {
            // WebSocket connection
            ws: null,
            connected: false,
            
            // CDN data
            cdnNodes: [],
            cdnStats: {},
            selectedCDNNode: '',
            
            // Proxy data
            proxyNodes: [],
            proxyStats: {},
            selectedProxyNode: '',
            
            // Logs
            logs: [],
            autoScroll: true,
            maxLogs: 1000,
            
            // API base URL
            apiBase: 'http://localhost:8080/api'
        };
    },
    
    mounted() {
        console.log('Vue.js application mounted');
        this.connectWebSocket();
        this.loadInitialData();
        this.startStatsPolling();
    },
    
    methods: {
        // WebSocket methods
        connectWebSocket() {
            this.ws = new WebSocket('ws://localhost:8080/ws');
            
            this.ws.onopen = () => {
                this.connected = true;
                this.addLog('INFO', 'SYSTEM', 'WebSocket connected');
            };
            
            this.ws.onmessage = (event) => {
                try {
                    const logEntry = JSON.parse(event.data);
                    this.addLog(logEntry.level, logEntry.service, logEntry.message, logEntry.timestamp);
                } catch (error) {
                    console.error('Failed to parse log entry:', error);
                }
            };
            
            this.ws.onclose = () => {
                this.connected = false;
                this.addLog('WARN', 'SYSTEM', 'WebSocket disconnected');
                // Reconnect after 5 seconds
                setTimeout(() => this.connectWebSocket(), 5000);
            };
            
            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.addLog('ERROR', 'SYSTEM', 'WebSocket error occurred');
            };
        },
        
        // API methods
        async loadInitialData() {
            await Promise.all([
                this.loadCDNNodes(),
                this.loadProxyNodes(),
                this.loadCDNStats(),
                this.loadProxyStats()
            ]);
        },
        
        async loadCDNNodes() {
            try {
                console.log('Loading CDN nodes...');
                const response = await fetch(`${this.apiBase}/cdn/nodes`);
                this.cdnNodes = await response.json();
                console.log('CDN nodes loaded:', this.cdnNodes);
            } catch (error) {
                console.error('Failed to load CDN nodes:', error);
            }
        },
        
        async loadProxyNodes() {
            try {
                const response = await fetch(`${this.apiBase}/proxy/nodes`);
                this.proxyNodes = await response.json();
            } catch (error) {
                console.error('Failed to load Proxy nodes:', error);
            }
        },
        
        async loadCDNStats() {
            try {
                const response = await fetch(`${this.apiBase}/cdn/stats`);
                this.cdnStats = await response.json();
            } catch (error) {
                console.error('Failed to load CDN stats:', error);
            }
        },
        
        async loadProxyStats() {
            try {
                const response = await fetch(`${this.apiBase}/proxy/stats`);
                this.proxyStats = await response.json();
            } catch (error) {
                console.error('Failed to load Proxy stats:', error);
            }
        },
        
        startStatsPolling() {
            setInterval(() => {
                this.loadCDNStats();
                this.loadProxyStats();
            }, 2000); // Poll every 2 seconds
        },
        
        // CDN methods
        async addCDNNode() {
            try {
                const response = await fetch(`${this.apiBase}/cdn/nodes`, {
                    method: 'POST'
                });
                const result = await response.json();
                await this.loadCDNNodes();
                this.addLog('INFO', 'SYSTEM', `Added CDN node: ${result.nodeId}`);
            } catch (error) {
                console.error('Failed to add CDN node:', error);
                this.addLog('ERROR', 'SYSTEM', 'Failed to add CDN node');
            }
        },
        
        async addCDNClient() {
            if (!this.selectedCDNNode) return;
            
            try {
                const response = await fetch(`${this.apiBase}/cdn/nodes/${this.selectedCDNNode}/clients`, {
                    method: 'POST'
                });
                const result = await response.json();
                await this.loadCDNNodes();
                this.addLog('INFO', 'SYSTEM', `Added client ${result.clientId} to CDN node ${this.selectedCDNNode}`);
            } catch (error) {
                console.error('Failed to add CDN client:', error);
                this.addLog('ERROR', 'SYSTEM', 'Failed to add CDN client');
            }
        },
        
        // Proxy methods
        async addProxyNode() {
            try {
                const response = await fetch(`${this.apiBase}/proxy/nodes`, {
                    method: 'POST'
                });
                const result = await response.json();
                await this.loadProxyNodes();
                this.addLog('INFO', 'SYSTEM', `Added Proxy node: ${result.nodeId}`);
            } catch (error) {
                console.error('Failed to add Proxy node:', error);
                this.addLog('ERROR', 'SYSTEM', 'Failed to add Proxy node');
            }
        },
        
        async addProxyClient() {
            if (!this.selectedProxyNode) return;
            
            try {
                const response = await fetch(`${this.apiBase}/proxy/nodes/${this.selectedProxyNode}/clients`, {
                    method: 'POST'
                });
                const result = await response.json();
                await this.loadProxyNodes();
                this.addLog('INFO', 'SYSTEM', `Added client ${result.clientId} to Proxy node ${this.selectedProxyNode}`);
            } catch (error) {
                console.error('Failed to add Proxy client:', error);
                this.addLog('ERROR', 'SYSTEM', 'Failed to add Proxy client');
            }
        },
        
        // Log methods
        addLog(level, service, message, timestamp = null) {
            const logEntry = {
                timestamp: timestamp || new Date().toISOString(),
                level: level,
                service: service,
                message: message
            };
            
            this.logs.unshift(logEntry);
            
            // Limit log entries
            if (this.logs.length > this.maxLogs) {
                this.logs = this.logs.slice(0, this.maxLogs);
            }
            
            // Auto-scroll to bottom
            if (this.autoScroll) {
                this.$nextTick(() => {
                    const container = this.$refs.logContainer;
                    if (container) {
                        container.scrollTop = container.scrollHeight;
                    }
                });
            }
        },
        
        clearLogs() {
            this.logs = [];
        },
        
        toggleAutoScroll() {
            this.autoScroll = !this.autoScroll;
        },
        
        // Utility methods
        formatTime(timestamp) {
            const date = new Date(timestamp);
            return date.toLocaleTimeString();
        },
        
        getLogClass(level) {
            return {
                'text-red-400': level === 'ERROR',
                'text-yellow-400': level === 'WARN',
                'text-blue-400': level === 'INFO',
                'text-gray-400': level === 'DEBUG'
            };
        },
        
        getServiceClass(service) {
            return {
                'text-blue-400': service === 'CDN',
                'text-purple-400': service === 'PROXY',
                'text-green-400': service === 'SYSTEM'
            };
        },
        
        getLevelClass(level) {
            return {
                'text-red-400': level === 'ERROR',
                'text-yellow-400': level === 'WARN',
                'text-blue-400': level === 'INFO',
                'text-gray-400': level === 'DEBUG'
            };
        }
    }
    }).mount('#app');
} catch (error) {
    console.error('Failed to initialize Vue app:', error);
    document.body.innerHTML = '<div style="padding: 20px; color: red;"><h1>Error: Failed to initialize Vue app</h1><p>' + error.message + '</p></div>';
}
