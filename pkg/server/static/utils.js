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