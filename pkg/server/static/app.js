let graphData = null
let nodePositions = {}
let draggedNode = null
let offsetX = 0, offsetY = 0
let scale = 1
let panX = 0, panY = 0
let isPanning = false
let panStartX = 0, panStartY = 0
let hoveredNode = null

document.addEventListener('DOMContentLoaded', () => {
    // Кнопки
    document.getElementById('btn-execute').onclick = execute
    document.getElementById('btn-parse').onclick = parseRepo
    loadTables()

    // Вкладки
    document.querySelectorAll('.tab').forEach(tab => {
        tab.onclick = () => switchTab(tab.dataset.tab)
    })

    // Canvas для графа
    const canvas = document.getElementById('graph-canvas')

    canvas.addEventListener('mousedown', (e) => {
        const pos = getMousePos(e)
        const node = findNodeAt(pos.x, pos.y)

        if (e.button === 0 && node) {
            draggedNode = node
            offsetX = pos.x - nodePositions[node.id].x
            offsetY = pos.y - nodePositions[node.id].y
        } else if (e.button === 2 || (e.button === 0 && !node)) {
            isPanning = true
            panStartX = e.clientX - panX
            panStartY = e.clientY - panY
        }
    })

    canvas.addEventListener('mousemove', (e) => {
        const pos = getMousePos(e)

        if (draggedNode) {
            nodePositions[draggedNode.id].x = pos.x - offsetX
            nodePositions[draggedNode.id].y = pos.y - offsetY
            drawGraph()
        } else if (isPanning) {
            panX = e.clientX - panStartX
            panY = e.clientY - panStartY
            drawGraph()
        } else {
            const node = findNodeAt(pos.x, pos.y)
            if (node !== hoveredNode) {
                hoveredNode = node
                canvas.style.cursor = node ? 'grab' : 'default'
                drawGraph()
            }
        }
    })

    canvas.addEventListener('mouseup', () => {
        draggedNode = null
        isPanning = false
    })

    canvas.addEventListener('wheel', (e) => {
        e.preventDefault()
        const zoom = e.deltaY > 0 ? 0.9 : 1.1
        scale *= zoom
        if (scale < 0.2) scale = 0.2
        if (scale > 5) scale = 5
        drawGraph()
    })

    canvas.addEventListener('contextmenu', (e) => e.preventDefault())
})

function switchTab(name) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'))
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'))
    document.querySelector(`.tab[data-tab="${name}"]`).classList.add('active')
    document.getElementById(`tab-${name}`).classList.add('active')
    if (name === 'graph') {
        if (!graphData) {
            loadGraph()
        } else {
            drawGraph()
        }
    }
}

async function parseRepo() {
    const path = document.getElementById('repo-path').value
    const status = document.getElementById('parse-status')
    status.textContent = '⏳ Анализируем...'

    const r = await fetch('/api/parse', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path })
    })
    const d = await r.json()

    if (d.error) {
        status.textContent = '❌ ' + d.error
    } else {
        status.textContent = `✅ ${d.files} файлов, ${d.classes} классов, ${d.functions} функций (${d.time_ms}ms)`
        loadGraph()
    }
}

async function loadGraph() {
    const r = await fetch('/api/graph')
    graphData = await r.json()
    initPositions()
    drawGraph()
}

function initPositions() {
    const nodes = graphData.nodes
    const cx = 450, cy = 300, radius = 200
    nodePositions = {}
    nodes.forEach((node, i) => {
        const angle = (2 * Math.PI * i) / nodes.length
        nodePositions[node.id] = {
            x: cx + radius * Math.cos(angle),
            y: cy + radius * Math.sin(angle),
        }
    })
}

function drawGraph() {
    if (!graphData || !graphData.nodes) return

    const canvas = document.getElementById('graph-canvas')
    const ctx = canvas.getContext('2d')
    ctx.clearRect(0, 0, canvas.width, canvas.height)

    ctx.save()
    ctx.translate(panX, panY)
    ctx.scale(scale, scale)

    const { nodes, edges } = graphData

    // Рёбра
    edges.forEach(edge => {
        const from = nodePositions[edge.from]
        const to = nodePositions[edge.to]
        if (!from || !to) return

        const isHovered = hoveredNode && (edge.from === hoveredNode.id || edge.to === hoveredNode.id)

        ctx.beginPath()
        ctx.strokeStyle = edge.type === 'calls'
            ? (isHovered ? '#ff6b81' : '#e94560')
            : (isHovered ? '#6a6a8a' : '#3a3a5a')
        ctx.lineWidth = isHovered ? 3 : 1.5
        ctx.moveTo(from.x, from.y)
        ctx.lineTo(to.x, to.y)
        ctx.stroke()

        // Стрелка
        const angle = Math.atan2(to.y - from.y, to.x - from.x)
        const arrowX = to.x - 22 * Math.cos(angle)
        const arrowY = to.y - 22 * Math.sin(angle)
        ctx.beginPath()
        ctx.fillStyle = ctx.strokeStyle
        ctx.moveTo(arrowX, arrowY)
        ctx.lineTo(
            arrowX - 8 * Math.cos(angle - 0.8),
            arrowY - 8 * Math.sin(angle - 0.8)
        )
        ctx.lineTo(
            arrowX - 8 * Math.cos(angle + 0.8),
            arrowY - 8 * Math.sin(angle + 0.8)
        )
        ctx.closePath()
        ctx.fill()
    })

    // Узлы
    nodes.forEach(node => {
        const pos = nodePositions[node.id]
        if (!pos) return

        const isHovered = hoveredNode && hoveredNode.id === node.id

        if (isHovered) {
            ctx.shadowColor = '#e94560'
            ctx.shadowBlur = 15
        }

        ctx.beginPath()
        ctx.fillStyle = node.type === 'class' ? '#0f3460' : '#16213e'
        ctx.strokeStyle = isHovered ? '#ff6b81' : '#e94560'
        ctx.lineWidth = isHovered ? 3 : 2
        ctx.arc(pos.x, pos.y, isHovered ? 24 : 20, 0, 2 * Math.PI)
        ctx.fill()
        ctx.stroke()
        ctx.shadowBlur = 0

        ctx.fillStyle = '#fff'
        ctx.font = isHovered ? 'bold 12px monospace' : '11px monospace'
        ctx.textAlign = 'center'
        ctx.fillText(node.label, pos.x, pos.y + 32)

        ctx.fillStyle = '#a0a0b0'
        ctx.font = '9px monospace'
        ctx.fillText(node.type, pos.x, pos.y - 26)
    })

    ctx.restore()

    // Легенда
    ctx.fillStyle = '#a0a0b0'
    ctx.font = '11px monospace'
    ctx.textAlign = 'left'
    ctx.fillText('🖱 Перетаскивай узлы | Колёсико — зум | ПКМ — панорама', 10, 20)
    ctx.fillText('🟣 class  🟢 function  ——— calls  ——— contains', 10, 38)
}

function getMousePos(e) {
    const canvas = document.getElementById('graph-canvas')
    const rect = canvas.getBoundingClientRect()
    return {
        x: (e.clientX - rect.left - panX) / scale,
        y: (e.clientY - rect.top - panY) / scale
    }
}

function findNodeAt(mx, my) {
    if (!graphData || !graphData.nodes) return null
    for (const node of graphData.nodes) {
        const pos = nodePositions[node.id]
        if (!pos) continue
        const dx = mx - pos.x
        const dy = my - pos.y
        if (dx * dx + dy * dy < 30 * 30) return node
    }
    return null
}

async function execute() {
    const sql = document.getElementById('sql-input').value.trim()
    if (!sql) return

    const r = await fetch('/api/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sql })
    })
    const d = await r.json()
    document.getElementById('result-output').textContent = d.result || d.error
}

async function loadTables() {
    const r = await fetch('/api/tables')
    const d = await r.json()
    document.getElementById('tables-list').innerHTML =
        (d.tables || []).map(t => `<div>📄 ${t}</div>`).join('')
}