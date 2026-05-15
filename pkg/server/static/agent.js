// static/agent.js
let currentAbortController = null;

// static/agent.js — исправленная версия

async function sendAgentMessage() {
    const msg = document.getElementById('agent-message').value.trim()
    if (!msg) return

    const session = parseInt(document.getElementById('agent-session').value)

    // Получаем выбранную модель из dropdown
    const modelSelect = document.getElementById('agent-model-select')
    const selectedModelId = modelSelect?.value

    if (!selectedModelId) {
        appendMessage('system', '❌ Модель не выбрана. Выберите модель в панели справа')
        return
    }

    console.log('selectedModelId:', selectedModelId, 'type:', typeof selectedModelId)

    // Загружаем модели из БД
    let data;
    try {
        data = await get('/api/config/models')
        console.log('Models response:', JSON.stringify(data, null, 2))
    } catch (err) {
        console.error('Failed to load models:', err)
        appendMessage('system', '❌ Не удалось загрузить список моделей')
        return
    }

    const models = data.models || []
    console.log('Models array:', models)
    console.log('Looking for model with id:', selectedModelId)

    // Пробуем найти модель (сравниваем как строки, так и числа)
    const selectedModel = models.find(m => {
        const match = String(m.id) === String(selectedModelId)
        if (match) {
            console.log('Found match:', m)
        }
        return match
    })

    if (!selectedModel) {
        console.error('Model not found. Available models:', models.map(m => ({ id: m.id, name: m.name })))
        appendMessage('system', `❌ Модель с ID ${selectedModelId} не найдена. Доступные модели: ${models.map(m => `${m.id}:${m.name}`).join(', ')}`)
        return
    }

    const model = selectedModel.name || ''
    const baseURL = selectedModel.base_url || ''
    const apiKey = selectedModel.api_key || ''

    console.log('Using model:', { model, baseURL, hasApiKey: !!apiKey })

    if (!model || !baseURL || !apiKey) {
        appendMessage('system', '❌ У модели не заполнены все поля (name, base_url, api_key)')
        return
    }

    // Отменяем предыдущий запрос
    if (currentAbortController) {
        currentAbortController.abort()
    }

    // Добавляем сообщение пользователя в чат
    appendMessage('user', msg)
    document.getElementById('agent-message').value = ''

    // Создаём контейнер для ответа
    const responseDiv = createStreamingMessage()

    currentAbortController = new AbortController()

    const requestBody = {
        session_id: session,
        message: msg,
        llm_key: apiKey,
        base_url: baseURL,
        model: model
    }

    console.log('Sending request:', { ...requestBody, llm_key: '***' })

    try {
        const response = await fetch('/api/agent/stream', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(requestBody),
            signal: currentAbortController.signal
        })

        if (!response.ok) {
            const errorText = await response.text()
            throw new Error(`HTTP ${response.status}: ${errorText}`)
        }

        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
            const { done, value } = await reader.read()
            if (done) break

            buffer += decoder.decode(value, { stream: true })
            const lines = buffer.split('\n')
            buffer = lines.pop() || ''

            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    try {
                        const eventData = JSON.parse(line.slice(6))
                        handleStreamEvent(eventData, responseDiv)
                    } catch (e) {
                        console.error('Parse error:', e, line)
                    }
                }
            }
        }

        responseDiv.classList.remove('streaming')
        loadContext()

    } catch (err) {
        console.error('Stream error:', err)
        if (err.name === 'AbortError') {
            appendMessage('system', '⏹ Генерация прервана')
            if (responseDiv.parentNode) responseDiv.remove()
        } else {
            if (responseDiv.parentNode) {
                responseDiv.classList.remove('streaming')
                const contentDiv = responseDiv.querySelector('.streaming-content')
                if (contentDiv) {
                    contentDiv.textContent = '❌ Ошибка: ' + err.message
                }
            } else {
                appendMessage('system', '❌ Ошибка: ' + err.message)
            }
        }
    } finally {
        currentAbortController = null
    }
}

function createStreamingMessage() {
    const chat = document.getElementById('agent-chat')
    const placeholder = chat.querySelector('.chat-placeholder')
    if (placeholder) placeholder.remove()

    const div = document.createElement('div')
    div.className = 'msg assistant streaming'
    div.innerHTML = '<div class="streaming-content"></div><div class="streaming-tools"></div>'
    chat.appendChild(div)
    chat.scrollTop = chat.scrollHeight

    console.log('Created streaming message div:', div)  // ← отладка
    return div
}

// static/agent.js — убедитесь, что handleStreamEvent правильно обрабатывает события

function handleStreamEvent(event, container) {
    console.log('handleStreamEvent:', event);  // ← добавить отладку

    const contentDiv = container.querySelector('.streaming-content')
    const toolsDiv = container.querySelector('.streaming-tools')

    if (!contentDiv) {
        console.error('No .streaming-content found in container')
        return
    }

    switch (event.type) {
        case 'chunk':
            // Добавляем текстовый чанк
            if (!contentDiv.textContent) {
                contentDiv.textContent = event.content
            } else {
                contentDiv.textContent += event.content
            }
            console.log('Chunk added, current text:', contentDiv.textContent)  // ← отладка
            break

        case 'tool_start':
            const toolStart = document.createElement('div')
            toolStart.className = 'tool-status pending'
            toolStart.textContent = event.content
            toolsDiv.appendChild(toolStart)
            break

        case 'tool_result':
            const lastTool = toolsDiv.lastChild
            if (lastTool && lastTool.classList.contains('pending')) {
                lastTool.classList.remove('pending')
                lastTool.classList.add('completed')
                lastTool.innerHTML = event.content.replace(/\n/g, '<br>')
            } else {
                const toolResult = document.createElement('div')
                toolResult.className = 'tool-result'
                toolResult.innerHTML = event.content.replace(/\n/g, '<br>')
                toolsDiv.appendChild(toolResult)
            }
            break

        case 'done':
            container.classList.remove('streaming')
            container.classList.add('assistant')
            console.log('Streaming done, final message:', contentDiv.textContent)  // ← отладка
            break

        case 'error':
            container.classList.remove('streaming')
            container.classList.add('system')
            contentDiv.textContent = '❌ ' + event.content
            break
    }

    // Скроллим к низу
    const chat = document.getElementById('agent-chat')
    chat.scrollTop = chat.scrollHeight
}

function stopAgentGeneration() {
    if (currentAbortController) {
        currentAbortController.abort()
        currentAbortController = null
        appendMessage('system', '⏹ Остановка генерации...')
    }
}

function appendMessage(type, text) {
    console.log('appendMessage called:', type, text.substring(0, 30))
    const chat = document.getElementById('agent-chat')
    const placeholder = chat.querySelector('.chat-placeholder')
    if (placeholder) placeholder.remove()

    const div = document.createElement('div')
    div.className = `msg ${type}`
    div.textContent = text
    chat.appendChild(div)
    chat.scrollTop = chat.scrollHeight
}

async function loadModels() {
    try {
        const data = await get('/api/config/models')
        const select = document.getElementById('agent-model-select')
        const modelInfo = document.getElementById('current-model-info')
        
        if (!select) return

        select.style.display = 'none'
        document.getElementById('add-model-form').style.display = 'none'
        if (modelInfo) modelInfo.textContent = ''

        if (!data.models || data.models.length === 0) {
            document.getElementById('add-model-form').style.display = 'block'
            if (modelInfo) modelInfo.textContent = 'Нет моделей. Создайте новую:'
            return
        }

        select.style.display = 'block'
        select.innerHTML = '<option value="">Выберите модель...</option>'

        data.models.forEach(model => {
            const option = document.createElement('option')
            option.value = model.id
            option.textContent = model.display_name || model.name
            select.appendChild(option)
        })

        const settings = await get('/api/config/settings')
        
        let selectedModel = null
        const activeId = settings.settings?.active_model_id
        if (activeId) {
            selectedModel = data.models.find(m => String(m.id) === String(activeId))
        }
        
        if (!selectedModel) {
            selectedModel = data.models.find(m => m.is_default)
        }
        
        if (!selectedModel) {
            selectedModel = data.models[0]
            await post('/api/config/models/active', { model_id: selectedModel.id })
        }

        select.value = selectedModel.id
        if (modelInfo) {
            modelInfo.textContent = selectedModel.base_url
        }
        
        select.onchange = async function() {
            if (!this.value || this.value === '') return
            const modelId = parseInt(this.value)
            await post('/api/config/models/active', { model_id: modelId })
            const model = data.models.find(m => String(m.id) === String(this.value))
            if (modelInfo && model) modelInfo.textContent = model.base_url
        }
        
    } catch (err) {
        console.error('Failed to load models:', err)
    }
}

async function loadProjects() {
    try {
        const data = await get('/api/config/projects')
        const select = document.getElementById('project-select')
        const projectInfo = document.getElementById('current-project-path')
        
        if (!select) return

        document.getElementById('add-project-form').style.display = 'none'
        if (projectInfo) projectInfo.textContent = ''

        if (!data.projects || data.projects.length === 0) {
            document.getElementById('add-project-form').style.display = 'block'
            if (projectInfo) projectInfo.textContent = 'Нет проектов. Создайте новый:'
            return
        }

        select.style.display = 'block'
        select.innerHTML = '<option value="">Выберите проект...</option>'

        data.projects.forEach(project => {
            const option = document.createElement('option')
            option.value = project.id
            option.textContent = project.name
            select.appendChild(option)
        })

        const settings = await get('/api/config/settings')
        let selectedProject = data.projects.find(p => String(p.id) === String(settings.settings?.active_project_id))
        
        if (!selectedProject) {
            selectedProject = data.projects[0]
            await post('/api/config/projects/active', { project_id: selectedProject.id })
        }

        select.value = selectedProject.id
        if (projectInfo) {
            projectInfo.textContent = selectedProject.root_path
        }
        
        select.onchange = async function() {
            if (!this.value) return
            await post('/api/config/projects/active', { project_id: parseInt(this.value) })
            const project = data.projects.find(p => String(p.id) === String(this.value))
            if (projectInfo && project) projectInfo.textContent = project.root_path
            document.getElementById('repo-path').value = project.root_path
            await parseRepo()
        }
        
    } catch (err) {
        console.error('Failed to load projects:', err)
    }
}

async function addModel() {
    console.log('addModel called')  // ← отладка

    const name = document.getElementById('new-model-name')?.value
    const displayName = document.getElementById('new-model-display')?.value
    const baseURL = document.getElementById('new-model-url')?.value
    const apiKey = document.getElementById('new-model-key')?.value
    const isDefault = document.getElementById('new-model-default')?.checked || false

    console.log('Form values:', { name, displayName, baseURL, hasApiKey: !!apiKey, isDefault })

    if (!name || !baseURL) {
        appendMessage('system', '❌ Заполните название и URL модели')
        return
    }

    try {
        const response = await fetch('/api/config/models/add', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: name.trim(),
                display_name: displayName?.trim() || name.trim(),
                base_url: baseURL.trim(),
                api_key: apiKey?.trim() || '',
                is_default: isDefault
            })
        })

        const data = await response.json()
        console.log('Add model response:', data)

        if (!response.ok || data.error) {
            throw new Error(data.error || `HTTP ${response.status}`)
        }

        // Очищаем форму
        document.getElementById('new-model-name').value = ''
        document.getElementById('new-model-display').value = ''
        document.getElementById('new-model-url').value = ''
        document.getElementById('new-model-key').value = ''
        document.getElementById('new-model-default').checked = false
        document.getElementById('add-model-form').style.display = 'none'

        // Перезагружаем список моделей
        await loadModels()
        appendMessage('system', `✅ Модель "${name}" добавлена`)

    } catch (err) {
        console.error('Add model error:', err)
        appendMessage('system', `❌ Ошибка: ${err.message}`)
    }
}

async function addProject() {
    const name = document.getElementById('new-project-name').value
    const rootPath = document.getElementById('new-project-path').value
    const description = document.getElementById('new-project-desc').value

    if (!name || !rootPath) {
        appendMessage('system', 'Заполните название и путь проекта')
        return
    }

    try {
        await post('/api/config/projects/add', {
            name, root_path: rootPath, description
        })

        // Очищаем форму
        document.getElementById('new-project-name').value = ''
        document.getElementById('new-project-path').value = ''
        document.getElementById('new-project-desc').value = ''
        document.getElementById('add-project-form').style.display = 'none'

        // Перезагружаем список
        loadProjects()
    } catch (err) {
        appendMessage('system', `❌ Ошибка: ${err.message}`)
    }
}

function toggleStreaming(enabled) {
    fetch('/api/config/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ streaming_enabled: enabled })
    }).catch(err => console.error('Failed to save streaming setting:', err))
}