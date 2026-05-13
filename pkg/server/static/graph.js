let graphData = null
let nodes = {}
let links = []
let draggedNode = null
let dragOffset = { x: 0, y: 0 }
let scale = 1
let pan = { x: 0, y: 0 }
let isPanning = false
let panStart = { x: 0, y: 0 }
let hovered = null
let selectedNode = null
let filterType = 'all'
let searchQuery = ''


function initGraph() {
    const cvs = document.getElementById('graph-canvas')
    resizeCanvas()
    window.addEventListener('resize', resizeCanvas)

    cvs.addEventListener('mousedown', onMouseDown)
    cvs.addEventListener('mousemove', onMouseMove)
    cvs.addEventListener('mouseup', onMouseUp)
    cvs.addEventListener('mouseleave', onMouseUp)
    cvs.addEventListener('wheel', onWheel, { passive: false })
    cvs.addEventListener('contextmenu', e => e.preventDefault())

    loadGraph()
}

function resizeCanvas() {
    const cvs = document.getElementById('graph-canvas')
    const container = cvs.parentElement
    cvs.width = container.clientWidth
    cvs.height = container.clientHeight
    if (graphData) renderGraph()
}

async function loadGraph() {
    try {
        graphData = await get('/api/graph')
        initNodes()
        runSimulation()
        renderGraph()
        updateFilesList()
    } catch (e) {
        console.error('Graph load error:', e)
    }
}

function initNodes() {
    nodes = {}
    links = []
    if (!graphData || !graphData.nodes) return

    const filtered = graphData.nodes.filter(n => {
        if (filterType !== 'all' && n.type !== filterType) return false
        if (searchQuery && !n.label.toLowerCase().includes(searchQuery)) return false
        return true
    })

    const ids = new Set(filtered.map(n => n.id))
    links = (graphData.edges || []).filter(e => ids.has(e.from) && ids.has(e.to))

    const cvs = document.getElementById('graph-canvas')
    const cols = Math.floor(Math.sqrt(filtered.length))
    filtered.forEach((n, i) => {
        const col = i % cols
        const row = Math.floor(i / cols)
        nodes[n.id] = {
            id: n.id,
            label: n.label,
            type: n.type,
            x: cvs.width * 0.1 + col * 150 + Math.random() * 30,
            y: cvs.height * 0.1 + row * 80 + Math.random() * 20,
            vx: 0, vy: 0
        }
    })
}

function runSimulation() {
    const nodeList = Object.values(nodes)
    for (let iter = 0; iter < 20; iter++) {
        nodeList.forEach(node => {
            if (draggedNode && node.id === draggedNode.id) return
            let fx = 0, fy = 0
            nodeList.forEach(other => {
                if (other.id === node.id) return
                const dx = node.x - other.x, dy = node.y - other.y
                const d = Math.sqrt(dx * dx + dy * dy) + 1
                if (d < 100) { fx += (dx / d) * 200 / (d * d); fy += (dy / d) * 200 / (d * d) }
            })
            links.forEach(link => {
                let other = null
                if (link.from === node.id) other = nodes[link.to]
                else if (link.to === node.id) other = nodes[link.from]
                if (!other) return
                fx += (other.x - node.x) * 0.01
                fy += (other.y - node.y) * 0.01
            })
            node.vx = (node.vx + fx) * 0.8
            node.vy = (node.vy + fy) * 0.8
            node.x += node.vx
            node.y += node.vy
        })
    }
}

function renderGraph() {
    const cvs = document.getElementById('graph-canvas')
    const ctx = cvs.getContext('2d')
    ctx.clearRect(0, 0, cvs.width, cvs.height)

    ctx.save()
    ctx.translate(pan.x, pan.y)
    ctx.scale(scale, scale)

    // Рёбра
    links.forEach(link => {
        const from = nodes[link.from], to = nodes[link.to]
        if (!from || !to) return
        ctx.beginPath()
        ctx.strokeStyle = link.type === 'calls' ? 'rgba(248,81,73,0.4)' : 'rgba(139,92,246,0.3)'
        ctx.lineWidth = 1
        ctx.moveTo(from.x, from.y)
        ctx.lineTo(to.x, to.y)
        ctx.stroke()
    })

    // Узлы
    Object.values(nodes).forEach(node => {
        const isHover = hovered && hovered.id === node.id
        const isSelected = selectedNode && selectedNode.id === node.id
        const r = isSelected ? 28 : isHover ? 24 : 20

        // Карточка
        const w = r * 3.5, h = r * 2
        ctx.fillStyle = '#161b22'
        ctx.strokeStyle = isSelected ? '#7c3aed' : isHover ? '#8b5cf6' : '#30363d'
        ctx.lineWidth = isSelected ? 2 : 1
        roundRect(ctx, node.x - w / 2, node.y - h / 2, w, h, 8)
        ctx.fill()
        ctx.stroke()

        // Иконка
        ctx.fillStyle = node.type === 'class' ? '#f85149' : node.type === 'call' ? '#d2991d' : '#7c3aed'
        ctx.font = '14px sans-serif'
        ctx.textAlign = 'center'
        ctx.fillText(node.type === 'class' ? '◆' : node.type === 'call' ? '▶' : '●', node.x, node.y - 2)

        // Название
        ctx.fillStyle = '#c9d1d9'
        ctx.font = '11px sans-serif'
        ctx.fillText(node.label.length > 15 ? node.label.slice(0, 13) + '..' : node.label, node.x, node.y + h / 2 - 4)
    })

    ctx.restore()
}

function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath()
    ctx.moveTo(x + r, y); ctx.lineTo(x + w - r, y)
    ctx.quadraticCurveTo(x + w, y, x + w, y + r)
    ctx.lineTo(x + w, y + h - r); ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h)
    ctx.lineTo(x + r, y + h); ctx.quadraticCurveTo(x, y + h, x, y + h - r)
    ctx.lineTo(x, y + r); ctx.quadraticCurveTo(x, y, x + r, y)
    ctx.closePath()
}

function fitGraph() { scale = 0.8; pan = { x: 50, y: 50 }; renderGraph() }
function resetGraph() { initNodes(); runSimulation(); renderGraph() }

function updateFilesList() {
    if (!graphData) return
    const files = graphData.nodes.filter(n => n.type === 'file')
    const list = document.getElementById('files-list')
    list.innerHTML = files.map(f => `<div class="list-item">📄 ${f.label}</div>`).join('')
}

// События мыши
function getPos(e) {
    const cvs = document.getElementById('graph-canvas')
    const r = cvs.getBoundingClientRect()
    return {
        x: (e.clientX - r.left - pan.x) / scale,
        y: (e.clientY - r.top - pan.y) / scale
    }
}

function findNode(x, y) {
    return Object.values(nodes).find(n => {
        const dx = x - n.x, dy = y - n.y
        return dx * dx + dy * dy < 900
    }) || null
}

function onMouseDown(e) {
    const pos = getPos(e)
    const node = findNode(pos.x, pos.y)
    if (e.button === 2 || (e.button === 0 && !node)) {
        isPanning = true
        panStart = { x: e.clientX - pan.x, y: e.clientY - pan.y }
    } else if (node) {
        draggedNode = node
        selectedNode = node
        dragOffset = { x: pos.x - node.x, y: pos.y - node.y }
        renderGraph()
    }
}

function onMouseMove(e) {
    const pos = getPos(e)
    if (draggedNode) {
        draggedNode.x = pos.x - dragOffset.x
        draggedNode.y = pos.y - dragOffset.y
        draggedNode.vx = 0; draggedNode.vy = 0
        renderGraph()
    } else if (isPanning) {
        pan.x = e.clientX - panStart.x
        pan.y = e.clientY - panStart.y
        renderGraph()
    } else {
        const node = findNode(pos.x, pos.y)
        if (node !== hovered) {
            hovered = node
            document.getElementById('graph-canvas').style.cursor = node ? 'pointer' : 'default'
            renderGraph()
        }
    }
}

function onMouseUp() { draggedNode = null; isPanning = false }
function onWheel(e) {
    e.preventDefault()
    scale *= e.deltaY > 0 ? 0.9 : 1.1
    scale = Math.max(0.1, Math.min(3, scale))
    renderGraph()
}

// Фильтры и поиск
function setupFilters() {
    document.querySelectorAll('.tag').forEach(tag => {
        tag.onclick = () => {
            document.querySelectorAll('.tag').forEach(t => t.classList.remove('active'))
            tag.classList.add('active')
            filterType = tag.dataset.filter
            initNodes()
            runSimulation()
            renderGraph()
        }
    })
}

function setupSearch() {
    document.getElementById('global-search').oninput = e => {
        searchQuery = e.target.value.toLowerCase()
        initNodes()
        runSimulation()
        renderGraph()
    }
}