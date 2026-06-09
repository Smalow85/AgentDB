// static/agent.js
window.currentAbortController = null;

// static/agent.js — исправленная версия

async function sendAgentMessage() {
    if (window.isGenerating) {
        appendMessage('system', '⏳ Уже генерирую ответ, подождите...');
        return;
    }

    const msg = document.getElementById('agent-message').value.trim();
    if (!msg) return;

    const sessionInput = document.getElementById('agent-session');
    // ✅ Валидация session_id
    let session = parseInt(sessionInput.value);
    if (isNaN(session) || session <= 0) {
        session = 1;
        sessionInput.value = '1';
    }

    const modelSelect = document.getElementById('agent-model-select');
    const selectedModelId = modelSelect?.value;

    if (!selectedModelId) {
        appendMessage('system', '❌ Модель не выбрана. Выберите модель в панели справа');
        return;
    }

    let data;
    try {
        data = await get('/api/config/models');
    } catch (err) {
        appendMessage('system', '❌ Не удалось загрузить список моделей: ' + err.message);
        return;
    }

    const models = data.models || [];
    const selectedModel = models.find(m => String(m.id) === String(selectedModelId));

    if (!selectedModel) {
        appendMessage('system', `❌ Модель с ID ${selectedModelId} не найдена`);
        return;
    }

    const model = selectedModel.name || '';
    const baseURL = selectedModel.base_url || '';
    const apiKey = selectedModel.api_key || '';

    if (!model || !baseURL || !apiKey) {
        appendMessage('system', '❌ У модели не заполнены все поля (name, base_url, api_key)');
        return;
    }

    // ✅ Отмена предыдущего с очисткой
    if (window.currentAbortController) {
        window.currentAbortController.abort();
        window.currentAbortController = null;
    }

    appendMessage('user', msg);
    document.getElementById('agent-message').value = '';

    const responseDiv = createStreamingMessage();
    window.currentAbortController = new AbortController();
    window.isGenerating = true;

    const stopBtn = document.getElementById('agent-stop-btn');
    if (stopBtn) stopBtn.style.display = 'inline-flex';

    const requestBody = {
        session_id: session,
        message: msg,
        llm_key: apiKey,
        base_url: baseURL,
        model: model
    };

    try {
        const response = await fetch('/api/agent/stream', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(requestBody),
            signal: window.currentAbortController.signal
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    try {
                        const eventData = JSON.parse(line.slice(6));
                        handleStreamEvent(eventData, responseDiv);
                    } catch (e) {
                        console.error('Parse error:', e);
                    }
                }
            }
        }

        responseDiv.classList.remove('streaming');
        if (typeof loadContext === 'function') loadContext();

    } catch (err) {
        console.error('Stream error:', err);
        if (err.name === 'AbortError') {
            appendMessage('system', '⏹ Генерация прервана');
            if (responseDiv.parentNode) responseDiv.remove();
        } else {
            if (responseDiv.parentNode) {
                responseDiv.classList.remove('streaming');
                const contentDiv = responseDiv.querySelector('.streaming-content');
                if (contentDiv) {
                    contentDiv.textContent = '❌ Ошибка: ' + err.message;
                }
            } else {
                appendMessage('system', '❌ Ошибка: ' + err.message);
            }
        }
    } finally {
        window.currentAbortController = null;
        window.isGenerating = false;
        if (stopBtn) stopBtn.style.display = 'none';
    }
}

function stopAgentGeneration() {
    if (window.currentAbortController) {
        window.currentAbortController.abort();
        window.currentAbortController = null;
        appendMessage('system', '⏹ Остановка генерации...');
    }
    window.isGenerating = false;
    const stopBtn = document.getElementById('agent-stop-btn');
    if (stopBtn) stopBtn.style.display = 'none';
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
    console.log('handleStreamEvent:', event);

    const contentDiv = container.querySelector('.streaming-content');
    const toolsDiv = container.querySelector('.streaming-tools');

    if (!contentDiv) {
        console.error('No .streaming-content found in container');
        return;
    }

    switch (event.type) {
        case 'chunk':
            if (!contentDiv.textContent) {
                contentDiv.textContent = event.content;
            } else {
                contentDiv.textContent += event.content;
            }
            break;

        case 'tool_start':
            const toolStart = document.createElement('div');
            toolStart.className = 'tool-status pending';
            toolStart.textContent = event.content;  // ✅ textContent безопасен
            toolsDiv.appendChild(toolStart);
            break;

        case 'tool_result':
            const lastTool = toolsDiv.lastChild;
            const toolResult = document.createElement('div');
            toolResult.className = 'tool-result';
            // ✅ Используем textContent + создаём <br> безопасно
            const lines = event.content.split('\n');
            for (let i = 0; i < lines.length; i++) {
                if (i > 0) toolResult.appendChild(document.createElement('br'));
                toolResult.appendChild(document.createTextNode(lines[i]));
            }

            if (lastTool && lastTool.classList && lastTool.classList.contains('pending')) {
                lastTool.classList.remove('pending');
                lastTool.classList.add('completed');
                lastTool.innerHTML = '';  // Очищаем
                for (let i = 0; i < lines.length; i++) {
                    if (i > 0) lastTool.appendChild(document.createElement('br'));
                    lastTool.appendChild(document.createTextNode(lines[i]));
                }
            } else {
                toolsDiv.appendChild(toolResult);
            }
            break;

        case 'done':
            container.classList.remove('streaming');
            container.classList.add('assistant');
            break;

        case 'error':
            container.classList.remove('streaming');
            container.classList.add('system');
            contentDiv.textContent = '❌ ' + event.content;
            break;
    }

    const chat = document.getElementById('agent-chat');
    chat.scrollTop = chat.scrollHeight;
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
        const data = await get('/api/config/models');
        const select = document.getElementById('agent-model-select');
        const modelInfo = document.getElementById('current-model-info');

        if (!select) return;

        if (!data.models || data.models.length === 0) {
            select.style.display = 'none';
            if (modelInfo) modelInfo.textContent = 'Нет моделей. Добавьте новую:';
            return;
        }

        select.style.display = 'block';

        // ✅ Сохраняем текущий выбор
        const currentValue = select.value;

        select.innerHTML = '<option value="">Выберите модель...</option>';
        data.models.forEach(model => {
            const option = document.createElement('option');
            option.value = model.id;
            option.textContent = model.display_name || model.name;
            select.appendChild(option);
        });

        // ✅ Восстанавливаем выбор, если он ещё существует
        let selectedModel = null;
        if (currentValue && data.models.some(m => String(m.id) === currentValue)) {
            select.value = currentValue;
            selectedModel = data.models.find(m => String(m.id) === currentValue);
        }

        if (!selectedModel) {
            const settings = await get('/api/config/settings');
            const activeId = settings.settings?.active_model_id;
            if (activeId) {
                selectedModel = data.models.find(m => String(m.id) === String(activeId));
            }
        }

        if (!selectedModel) {
            selectedModel = data.models.find(m => m.is_default);
        }

        if (!selectedModel && data.models.length > 0) {
            selectedModel = data.models[0];
            await post('/api/config/models/active', { model_id: selectedModel.id });
        }

        if (selectedModel) {
            select.value = selectedModel.id;
            if (modelInfo) {
                modelInfo.textContent = selectedModel.base_url;
            }
        }

        // ✅ Сохраняем обработчик
        select.onchange = async function () {
            if (!this.value || this.value === '') return;
            const modelId = parseInt(this.value);
            await post('/api/config/models/active', { model_id: modelId });
            const model = data.models.find(m => String(m.id) === String(this.value));
            if (modelInfo && model) modelInfo.textContent = model.base_url;
        };

    } catch (err) {
        console.error('Failed to load models:', err);
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