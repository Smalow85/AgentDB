async function sendAgentMessage() {
    const msg = document.getElementById('agent-message').value.trim()
    if (!msg) return

    const session = parseInt(document.getElementById('agent-session').value)
    const key = document.getElementById('agent-key').value
    const url = document.getElementById('agent-url').value
    const model = document.getElementById('agent-model').value

    if (!key || !url) {
        appendMessage('system', 'Настройте API Key и Base URL в панели справа')
        return
    }

    appendMessage('user', msg)
    document.getElementById('agent-message').value = ''
    appendMessage('system', '⏳ Думает...')

    try {
        const d = await post('/api/agent/loop', {
            session_id: session,
            message: msg,
            llm_key: key,
            base_url: url,
            model: model
        })

        // Удаляем "Думает..."
        const chat = document.getElementById('agent-chat')
        chat.removeChild(chat.lastChild)

        if (d.error) {
            appendMessage('system', 'Ошибка: ' + d.error)
        } else {
            appendMessage('assistant', d.result)
        }

        loadContext()
    } catch (err) {
        const chat = document.getElementById('agent-chat')
        chat.removeChild(chat.lastChild)
        appendMessage('system', 'Ошибка соединения')
    }
}

function appendMessage(type, text) {
    const chat = document.getElementById('agent-chat')

    // Убираем плейсхолдер
    const placeholder = chat.querySelector('.chat-placeholder')
    if (placeholder) placeholder.remove()

    const div = document.createElement('div')
    div.className = `msg ${type}`
    div.textContent = text
    chat.appendChild(div)
    chat.scrollTop = chat.scrollHeight
}