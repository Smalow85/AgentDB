// utils.js — должно быть только одно объявление каждой функции
async function api(url, opts = {}) {
    const res = await fetch(url, {
        headers: { 'Content-Type': 'application/json', ...opts.headers },
        ...opts
    })
    return res.json()
}

async function get(url) { return api(url) }
async function post(url, body) { return api(url, { method: 'POST', body: JSON.stringify(body) }) }

function escapeHTML(s) {
    const d = document.createElement('div')
    d.textContent = s
    return d.innerHTML
}

function formatBytes(n) {
    if (n < 1024) return n + ' B'
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
    return (n / 1048576).toFixed(1) + ' MB'
}

function formatDuration(ms) {
    if (ms < 1000) return ms + 'ms'
    return (ms / 1000).toFixed(2) + 's'
}

function setExample(text) {
    document.getElementById('agent-message').value = text
    document.getElementById('agent-message').focus()
    sendAgentMessage()
}

function clearChat() {
    const chat = document.getElementById('agent-chat')
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
    `
}

// Флаг для отслеживания генерации
window.isGenerating = false

// Обёртка для sendAgentMessage
const originalSend = window.sendAgentMessage
window.sendAgentMessage = async function () {
    if (window.isGenerating) {
        appendMessage('system', '⏳ Уже генерирую ответ, подождите...')
        return
    }

    const stopBtn = document.getElementById('agent-stop-btn')
    if (stopBtn) stopBtn.style.display = 'inline-flex'
    window.isGenerating = true

    try {
        await originalSend()
    } finally {
        if (stopBtn) stopBtn.style.display = 'none'
        window.isGenerating = false
    }
}