document.addEventListener('DOMContentLoaded', () => {
    // State
    let currentConfig = {};
    let configSchema = {};
    let events = [];

    // Tabs
    const tabs = document.querySelectorAll('.nav-links li[data-tab]');
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            
            const target = tab.getAttribute('data-tab');
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            document.getElementById(`tab-${target}`).classList.add('active');
            document.getElementById('page-title').innerText = tab.innerText.split(' ')[1];
        });
    });

    // Fetch Initial Data
    fetchStatus();
    fetchConfig();
    fetchActivity();

    // Poll for updates
    setInterval(fetchStatus, 5000);
    setInterval(fetchActivity, 2000);

    // --- Data Fetching ---

    async function fetchStatus() {
        try {
            const res = await fetch('/api/status');
            const data = await res.json();
            document.getElementById('uptime-val').innerText = data.uptime;
            // update metrics counters here
        } catch (e) { console.error('Status fetch failed', e); }
    }

    async function fetchConfig() {
        try {
            const [configRes, schemaRes] = await Promise.all([
                fetch('/api/config'),
                fetch('/api/config/schema')
            ]);
            currentConfig = await configRes.json();
            configSchema = await schemaRes.json();
            renderConfigForm();
        } catch (e) { console.error('Config fetch failed', e); }
    }

    async function fetchActivity() {
        try {
            const res = await fetch('/api/activity');
            const data = await res.json();
            if (data.length !== events.length) {
                events = data;
                renderActivity();
            }
        } catch (e) { console.error('Activity fetch failed', e); }
    }

    // --- Rendering ---

    function renderActivity() {
        const feed = document.getElementById('activity-feed');
        feed.innerHTML = events.reverse().map(event => `
            <div class="event ${event.type}">
                <div class="event-header">
                    <span class="event-type">${event.type.toUpperCase()} | ${event.channel}</span>
                    <span class="event-time">${new Date(event.time).toLocaleTimeString()}</span>
                </div>
                <div class="event-content">${escapeHTML(event.content)}</div>
            </div>
        `).join('');
    }

    function renderConfigForm() {
        const container = document.getElementById('config-form-container');
        container.innerHTML = '';
        
        // Simple recursive renderer for the schema
        const form = createFormSection(currentConfig, configSchema.properties, "");
        container.appendChild(form);
    }

    function createFormSection(data, properties, path) {
        const div = document.createElement('div');
        div.className = 'config-section';
        
        for (const [key, prop] of Object.entries(properties)) {
            const fullPath = path ? `${path}.${key}` : key;
            const value = data ? data[key] : null;

            if (prop.type === 'object' && prop.properties) {
                const group = document.createElement('div');
                group.className = 'form-group-cluster';
                group.innerHTML = `<h4>${key}</h4>`;
                group.appendChild(createFormSection(value, prop.properties, fullPath));
                div.appendChild(group);
            } else {
                const group = document.createElement('div');
                group.className = 'form-group';
                
                let input;
                if (prop.type === 'boolean') {
                    input = `<input type="checkbox" id="cfg-${fullPath}" ${value ? 'checked' : ''}>`;
                } else if (prop.type === 'number') {
                    input = `<input type="number" id="cfg-${fullPath}" value="${value || 0}">`;
                } else {
                    input = `<input type="text" id="cfg-${fullPath}" value="${escapeHTML(value || '')}">`;
                }

                group.innerHTML = `
                    <label for="cfg-${fullPath}">${key}</label>
                    <span class="description">${prop.description || ''}</span>
                    ${input}
                `;
                div.appendChild(group);
            }
        }
        return div;
    }

    // --- Helpers ---
    function escapeHTML(str) {
        if (!str) return '';
        return str.replace(/[&<>"']/g, m => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        })[m]);
    }

    // Restart button
    document.getElementById('restart-btn').addEventListener('click', async () => {
        if (confirm('Are you sure you want to restart the gateway? All active connections will be dropped.')) {
            try {
                await fetch('/api/restart', { method: 'POST' });
                alert('Restarting... Please refresh in a few seconds.');
            } catch (e) { alert('Restart failed'); }
        }
    });
});
