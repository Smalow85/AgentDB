let graphData = null
let nodes = {}
let links = []
let draggedNode = null
let dragOffset = {x:0, y:0}
let scale = 1
let pan = {x:0, y:0}
let isPanning = false
let panStart = {x:0, y:0}
let hovered = null
let selected = null
let filterType = 'all'
let searchQuery = ''
let simulationTick = 0

document.addEventListener('DOMContentLoaded', () => {
    document.getElementById('btn-execute').onclick = execute
    document.getElementById('btn-parse').onclick = parseRepo
    
    document.querySelectorAll('.filter-btn').forEach(btn => {
        btn.onclick = () => {
            document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'))
            btn.classList.add('active')
            filterType = btn.dataset.filter
            runSimulation()
        }
    })
    
    document.getElementById('search-input').oninput = e => {
        searchQuery = e.target.value.toLowerCase()
        draw()
    }
    
    loadTables()
    document.querySelectorAll('.tab').forEach(t => t.onclick = () => switchTab(t.dataset.tab))

    const cvs = document.getElementById('graph-canvas')
    
    window.addEventListener('resize', () => {
        if (graphData) draw()
    })
    
    cvs.addEventListener('mousedown', e => {
        const pos = getPos(e, cvs)
        const node = findNode(pos.x, pos.y)
        
        if (e.button === 0 && node) {
            draggedNode = node
            dragOffset = {x: pos.x - node.x, y: pos.y - node.y}
        } else if (e.button === 2 || (e.button === 0 && !node)) {
            isPanning = true
            panStart = {x: e.clientX - pan.x, y: e.clientY - pan.y}
        }
    })

    cvs.addEventListener('mousemove', e => {
        const pos = getPos(e, cvs)
        
        if (draggedNode) {
            draggedNode.x = pos.x - dragOffset.x
            draggedNode.y = pos.y - dragOffset.y
            draggedNode.vx = 0
            draggedNode.vy = 0
            draw()
        } else if (isPanning) {
            pan.x = e.clientX - panStart.x
            pan.y = e.clientY - panStart.y
            draw()
        } else {
            const node = findNode(pos.x, pos.y)
            if (node !== hovered) {
                hovered = node
                cvs.style.cursor = node ? 'grab' : 'default'
                draw()
            }
        }
    })

    cvs.addEventListener('mouseup', e => {
        if (draggedNode && !isPanning) {
            selected = draggedNode
            showNodeDetails(selected)
            draw()
        }
        draggedNode = null
        isPanning = false
    })
    cvs.addEventListener('mouseleave', () => { draggedNode = null; isPanning = false; hovered = null; draw() })

    cvs.addEventListener('wheel', e => {
        e.preventDefault()
        scale *= e.deltaY > 0 ? 0.9 : 1.1
        scale = Math.max(0.1, Math.min(3, scale))
        draw()
    })
    
    cvs.addEventListener('contextmenu', e => e.preventDefault())
})

function switchTab(name) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'))
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'))
    document.querySelector('.tab[data-tab="'+name+'"]').classList.add('active')
    document.getElementById('tab-'+name).classList.add('active')
    if (name === 'graph' && !graphData) loadGraph()
}

async function loadGraph() {
    graphData = await (await fetch('/api/graph')).json()
    initNodes()
    runSimulation()
    draw()
    console.log('Loaded', Object.keys(nodes).length, 'nodes, canvas:', document.getElementById('graph-canvas').width, 'x', document.getElementById('graph-canvas').height)
}

function initNodes() {
    const cvs = document.getElementById('graph-canvas')
    const cw = cvs.width
    const ch = cvs.height
    const filtered = getFilteredNodes()
    const ids = new Set(filtered.map(n => n.id))
    links = graphData.edges.filter(e => ids.has(e.from) && ids.has(e.to))
    
    filtered.forEach((n, i) => {
        const col = i % 15
        const row = Math.floor(i / 15)
        nodes[n.id] = {
            id: n.id,
            label: n.label,
            type: n.type,
            x: 80 + col * 55 + Math.random() * 20,
            y: 80 + row * 55 + Math.random() * 20,
            vx: 0,
            vy: 0
        }
    })
}

function runSimulation() {
    if (!graphData) return
    
    const cvs = document.getElementById('graph-canvas')
    const cw = cvs.width
    const ch = cvs.height
    
    const filtered = getFilteredNodes()
    const nodeList = Object.values(nodes)
    
    for (let iter = 0; iter < 30; iter++) {
        nodeList.forEach(node => {
            if (draggedNode && node.id === draggedNode.id) return
            
            let fx = 0, fy = 0
            
            nodeList.forEach(other => {
                if (other.id === node.id) return
                const dx = node.x - other.x
                const dy = node.y - other.y
                const d = Math.sqrt(dx*dx + dy*dy) + 0.1
                if (d < 120) {
                    fx += (dx / d) * 300 / (d * d)
                    fy += (dy / d) * 300 / (d * d)
                }
            })
            
            links.forEach(link => {
                let other = null
                if (link.from === node.id) other = nodes[link.to]
                else if (link.to === node.id) other = nodes[link.from]
                if (!other) return
                
                const dx = other.x - node.x
                const dy = other.y - node.y
                fx += dx * 0.008
                fy += dy * 0.008
            })
            
            fx += (cw/2 - node.x) * 0.003
            fy += (ch/2 - node.y) * 0.003
            
            node.vx = (node.vx + fx) * 0.85
            node.vy = (node.vy + fy) * 0.85
            node.x += node.vx
            node.y += node.vy
            
            node.x = Math.max(40, Math.min(cw-40, node.x))
            node.y = Math.max(40, Math.min(ch-40, node.y))
        })
    }
}

function getFilteredNodes() {
    if (!graphData || !graphData.nodes) return []
    return graphData.nodes.filter(n => {
        if (filterType !== 'all' && n.type !== filterType) return false
        if (searchQuery && !n.label.toLowerCase().includes(searchQuery)) return false
        return true
    })
}

function draw() {
    const cvs = document.getElementById('graph-canvas')
    const ctx = cvs.getContext('2d')
    ctx.clearRect(0, 0, cvs.width, cvs.height)
    
    ctx.save()
    ctx.translate(pan.x, pan.y)
    ctx.scale(scale, scale)

    const nodeList = Object.values(nodes)
    const filteredIds = new Set(getFilteredNodes().map(n => n.id))
    
    // Status
    ctx.fillStyle = '#707090'
    ctx.font = '11px sans-serif'
    ctx.fillText(nodeList.length + ' nodes | ' + links.length + ' links', 10, 18)

    // Links
    links.forEach(link => {
        const from = nodes[link.from]
        const to = nodes[link.to]
        if (!from || !to) return
        
        ctx.beginPath()
        ctx.strokeStyle = link.type === 'calls' ? '#944' : '#446'
        ctx.lineWidth = 0.8
        ctx.moveTo(from.x, from.y)
        ctx.lineTo(to.x, to.y)
        ctx.stroke()
    })

    // Nodes
    nodeList.forEach(node => {
        const isHover = hovered && hovered.id === node.id
        const isSelected = selected && selected.id === node.id
        const isSearch = searchQuery && node.label.toLowerCase().includes(searchQuery)
        
        ctx.beginPath()
        const colors = { file: '#4a4', class: '#c42', function: '#38c', call: '#949' }
        ctx.fillStyle = colors[node.type] || '#38c'
        ctx.strokeStyle = isSelected ? '#ff0' : (isHover ? '#f64' : (isSearch ? '#fa0' : '#e54'))
        ctx.lineWidth = isSelected ? 3 : (isHover ? 2.5 : 1.5)
        ctx.arc(node.x, node.y, isSelected ? 11 : (isHover ? 10 : 7), 0, Math.PI * 2)
        ctx.fill()
        ctx.stroke()
        
        // Label
        ctx.fillStyle = (isSelected || isHover) ? '#fff' : '#aaa'
        ctx.font = '9px sans-serif'
        ctx.textAlign = 'center'
        const label = node.label.length > 14 ? node.label.slice(0,11)+'...' : node.label
        ctx.fillText(label, node.x, node.y + 18)
    })

    ctx.restore()
    
    // Footer
    ctx.fillStyle = '#556'
    ctx.font = '10px sans-serif'
    ctx.fillText('Click node: details | Drag: move | Scroll: zoom | Right-drag: pan', 8, cvs.height - 10)
}

function getPos(e, cvs) {
    const r = cvs.getBoundingClientRect()
    const scaleX = cvs.width / r.width
    const scaleY = cvs.height / r.height
    return {
        x: ((e.clientX - r.left) * scaleX - pan.x) / scale,
        y: ((e.clientY - r.top) * scaleY - pan.y) / scale
    }
}

function findNode(x, y) {
    const list = Object.values(nodes)
    for (const n of list) {
        const dx = x - n.x
        const dy = y - n.y
        if (dx*dx + dy*dy < 150) return n
    }
    return null
}

function showNodeDetails(node) {
    const section = document.getElementById('node-details-section')
    const details = document.getElementById('node-details')
    
    if (!node) {
        section.style.display = 'none'
        return
    }
    
    section.style.display = 'block'
    
    let propsHtml = ''
    if (node.props) {
        for (const [key, val] of Object.entries(node.props)) {
            if (key !== 'name') {
                propsHtml += `<div class="prop"><span class="prop-key">${key}:</span> <span class="prop-val">${val}</span></div>`
            }
        }
    }
    
    const connected = links.filter(l => l.from === node.id || l.to === node.id)
    let connectionsHtml = ''
    connected.forEach(link => {
        const otherId = link.from === node.id ? link.to : link.from
        const other = nodes[otherId]
        const direction = link.from === node.id ? '→' : '←'
        if (other) {
            connectionsHtml += `<div class="connection">${direction} ${link.type} ${other.label}</div>`
        }
    })
    
    details.innerHTML = `
        <div class="node-name">${node.label}</div>
        <div class="node-type">${node.type}</div>
        <div class="node-id">ID: ${node.id}</div>
        ${propsHtml ? '<div class="props-section">' + propsHtml + '</div>' : ''}
        ${connectionsHtml ? '<div class="connections-section"><h4>Connections</h4>' + connectionsHtml + '</div>' : ''}
    `
}

async function parseRepo() {
    const status = document.getElementById('parse-status')
    status.textContent = 'Analyzing...'
    const d = await (await fetch('/api/parse', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({path: document.getElementById('repo-path').value})
    })).json()
    
    if (d.error) status.textContent = 'Error: ' + d.error
    else { status.textContent = d.files + ' files, ' + d.classes + ' classes' ; loadGraph() }
}

async function execute() {
    const sql = document.getElementById('sql-input').value.trim()
    if (!sql) return
    const d = await (await fetch('/api/query', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({sql})
    })).json()
    document.getElementById('result-output').textContent = d.result || d.error
}

async function loadTables() {
    const d = await (await fetch('/api/tables')).json()
    document.getElementById('tables-list').innerHTML = (d.tables || []).map(t => '<div>'+t+'</div>').join('')
}