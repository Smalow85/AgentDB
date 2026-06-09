// utils.js — исправленная версия

async function api(url, opts = {}) {
    const res = await fetch(url, {
        headers: { 'Content-Type': 'application/json', ...opts.headers },
        ...opts
    });

    if (!res.ok) {
        const text = await res.text();
        throw new Error(`HTTP ${res.status}: ${text.substring(0, 100)}`);
    }

    return res.json();
}

async function get(url) {
    return api(url);
}

async function post(url, body) {
    return api(url, { method: 'POST', body: JSON.stringify(body) });
}

function escapeHTML(s) {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
}

function formatBytes(n) {
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    return (n / 1048576).toFixed(1) + ' MB';
}

function formatDuration(ms) {
    if (ms < 1000) return ms + 'ms';
    return (ms / 1000).toFixed(2) + 's';
}

function setExample(text) {
    const input = document.getElementById('agent-message');
    if (input) {
        input.value = text;
        input.focus();
        if (typeof sendAgentMessage === 'function') {
            sendAgentMessage();
        }
    }
}

// ✅ Очистка чата с сохранением session_id
function clearChat() {
    const chat = document.getElementById('agent-chat');
    const sessionId = document.getElementById('agent-session')?.value || '1';

    chat.innerHTML = `
        <div class="chat-placeholder">
            <div class="logo-large">⬡</div>
            <p>AgentDB Assistant</p>
            <p class="muted">Задайте вопрос агенту — он поможет анализировать код</p>
            <div class="example-questions">
                <button class="example-btn" onclick="setExample('Покажи все классы в проекте')">📦 Показать классы</button>
                <button class="example-btn" onclick="setExample('Найди все вызовы функции main')">🔍 Найти вызовы</button>
                <button class="example-btn" onclick="setExample('Создай новый файл test.go')">📝 Создать файл</button>
            </div>
        </div>
    `;

    // Сброс стриминга
    if (window.currentAbortController) {
        window.currentAbortController.abort();
        window.currentAbortController = null;
    }
    window.isGenerating = false;

    const stopBtn = document.getElementById('agent-stop-btn');
    if (stopBtn) stopBtn.style.display = 'none';
}

window.isGenerating = false;