let clicks = parseInt(localStorage.getItem('referralClicks') || '0')
let currentChain = 'Ethereum'
let allPools = []
let allProtocols = []
let apyChart = null

const REFRESH_INTERVAL = 5 * 60 * 1000

document.getElementById('clicks').textContent = clicks
document.getElementById('analyticsClicks').textContent = clicks

// --- WebAuthn Helpers ---
function bufferToBase64url(buffer) {
  const bytes = new Uint8Array(buffer)
  let str = ''
  for (let i = 0; i < bytes.byteLength; i++) str += String.fromCharCode(bytes[i])
  return btoa(str).replace(/+/g, '-').replace(///g, '_').replace(/=+$/, '')
}

function base64urlToBuffer(base64url) {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const pad = base64.length % 4 ? '='.repeat(4 - (base64.length % 4)) : ''
  const binary = atob(base64 + pad)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

// --- Auth Logic ---
async function checkSession() {
  try {
    const res = await fetch('/api/session')
    if (res.ok) {
      const data = await res.json()
      showAuthenticatedUI(data.email)
    }
  } catch (err) {
    console.log('No active session')
  }
}

function showAuthenticatedUI(email) {
  document.getElementById('authStatus').innerHTML = `Signed in as <strong>${email}</strong>`
  document.getElementById('authActions').innerHTML = '<button class="btn btn-secondary" id="logoutBtn">Sign Out</button>'
  document.getElementById('logoutBtn').addEventListener('click', logout)
}

function showUnauthenticatedUI() {
  document.getElementById('authStatus').textContent = 'Sign in to save your preferences'
  document.getElementById('authActions').innerHTML = `
    <input type="email" id="authEmail" placeholder="Enter your email">
    <button class="btn" id="registerBtn">Register</button>
    <button class="btn btn-secondary" id="loginBtn">Sign In</button>
  `
  document.getElementById('registerBtn').addEventListener('click', register)
  document.getElementById('loginBtn').addEventListener('click', login)
}

async function register() {
  const email = document.getElementById('authEmail').value.trim()
  if (!email) return alert('Please enter an email')

  try {
    const res = await fetch('/api/register/options', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email })
    })
    
    if (!res.ok) {
      const err = await res.text()
      alert(err)
      return
    }

    const data = await res.json()
    const publicKey = data.publicKey.publicKey

    publicKey.challenge = base64urlToBuffer(publicKey.challenge)
    publicKey.user.id = base64urlToBuffer(publicKey.user.id)

    const credential = await navigator.credentials.create({ publicKey })

    const attestationResponse = {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64url(credential.response.attestationObject),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)
      }
    }

    const verifyRes = await fetch(`/api/register/verify?sessionID=${data.sessionID}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(attestationResponse)
    })

    if (verifyRes.ok) {
      showAuthenticatedUI(email)
    } else {
      alert('Registration failed')
    }
  } catch (err) {
    console.error(err)
    alert('Registration error')
  }
}

async function login() {
  const email = document.getElementById('authEmail').value.trim()
  if (!email) return alert('Please enter an email')

  try {
    const res = await fetch('/api/login/options', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email })
    })

    if (!res.ok) {
      const err = await res.text()
      alert(err)
      return
    }

    const data = await res.json()
    const publicKey = data.publicKey.publicKey

    publicKey.challenge = base64urlToBuffer(publicKey.challenge)
    if (publicKey.allowCredentials) {
      publicKey.allowCredentials = publicKey.allowCredentials.map(c => ({
        ...c,
        id: base64urlToBuffer(c.id)
      }))
    }

    const assertion = await navigator.credentials.get({ publicKey })

    const assertionResponse = {
      id: assertion.id,
      rawId: bufferToBase64url(assertion.rawId),
      type: assertion.type,
      response: {
        authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
        clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
        signature: bufferToBase64url(assertion.response.signature),
        userHandle: assertion.response.userHandle ? bufferToBase64url(assertion.response.userHandle) : null
      }
    }

    const verifyRes = await fetch(`/api/login/verify?sessionID=${data.sessionID}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(assertionResponse)
    })

    if (verifyRes.ok) {
      showAuthenticatedUI(email)
    } else {
      alert('Authentication failed')
    }
  } catch (err) {
    console.error(err)
    alert('Login error')
  }
}

async function logout() {
  await fetch('/api/logout', { method: 'POST' })
  showUnauthenticatedUI()
}

// --- DeFi Data Logic ---
async function fetchData() {
  const [protocolsRes, yieldsRes] = await Promise.all([
    fetch('https://api.llama.fi/protocols'),
    fetch('https://yields.llama.fi/pools')
  ])
  allProtocols = await protocolsRes.json()
  const yieldsData = await yieldsRes.json()
  allPools = yieldsData.data
}

function renderChain(chain, silent) {
  const list = document.getElementById('protocolList')
  list.innerHTML = ''
  if (!silent) document.getElementById('loading').style.display = 'block'

  const chainPools = allPools
    .filter(p => p.chain === chain && p.tvlUsd > 1000000 && p.apy > 0 && p.apy < 100)
    .sort((a, b) => b.tvlUsd - a.tvlUsd)

  const seen = new Set()
  const protocols = []
  chainPools.forEach(p => {
    if (!seen.has(p.project) && protocols.length < 15) {
      seen.add(p.project)
      const best = chainPools.filter(x => x.project === p.project)
        .sort((a, b) => b.apy - a.apy)[0]
      const meta = allProtocols.find(m =>
        m.slug === p.project || m.name.toLowerCase().replace(/[^a-z0-9]/g, '') === p.project.replace(/[^a-z0-9]/g, '')
      )
      protocols.push({
        name: meta ? meta.name : p.project,
        category: meta ? meta.category : 'DeFi',
        tvl: chainPools.filter(x => x.project === p.project).reduce((s, x) => s + x.tvlUsd, 0),
        apy: best.apy,
        url: meta && meta.url ? meta.url : 'https://defillama.com/protocol/' + p.project
      })
    }
  })

  document.getElementById('total').textContent = protocols.length
  document.getElementById('bestApy').textContent =
    (protocols.length ? Math.max(...protocols.map(p => p.apy)) : 0).toFixed(1) + '%'
  document.getElementById('totalTvl').textContent =
    '$' + (protocols.reduce((s, p) => s + p.tvl, 0) / 1e9).toFixed(2) + 'B'

  renderChart(protocols.slice(0, 10))

  protocols.forEach(p => {
    const li = document.createElement('li')
    li.className = 'protocol-item'
    li.innerHTML = `
      <div class="protocol-info">
        <h3></h3>
        <div class="category"></div>
        <div class="tvl"></div>
      </div>
      <div>
        <div class="protocol-apy"></div>
        <button class="btn">Deposit</button>
      </div>
    `
    li.querySelector('h3').textContent = p.name
    li.querySelector('.category').textContent = p.category
    li.querySelector('.tvl').textContent = 'TVL: $' + (p.tvl / 1e6).toFixed(1) + 'M'
    li.querySelector('.protocol-apy').textContent = p.apy.toFixed(2) + '% APY'
    li.querySelector('button').addEventListener('click', () => deposit(p.name, p.url))
    list.appendChild(li)
  })

  document.getElementById('loading').style.display = 'none'
}

function renderChart(protocols) {
  const ctx = document.getElementById('apyChart').getContext('2d')
  if (apyChart) apyChart.destroy()

  apyChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: protocols.map(p => p.name),
      datasets: [{
        label: 'APY %',
        data: protocols.map(p => p.apy),
        backgroundColor: 'rgba(99, 102, 241, 0.6)',
        borderColor: 'rgba(99, 102, 241, 1)',
        borderWidth: 1,
        borderRadius: 4,
      }]
    },
    options: {
      indexAxis: 'y',
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { beginAtZero: true, ticks: { color: '#64748b' }, grid: { color: 'rgba(100, 116, 139, 0.1)' } },
        y: { ticks: { color: '#e2e8f0' }, grid: { display: false } }
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: '#1e293b', titleColor: '#e2e8f0', bodyColor: '#e2e8f0',
          callbacks: { label: ctx => ctx.parsed.x.toFixed(2) + '% APY' }
        }
      }
    }
  })
}

async function refreshData(silent) {
  try {
    await fetchData()
    renderChain(currentChain, silent)
    const now = new Date()
    document.getElementById('lastUpdated').textContent =
      'Last updated: ' + now.toLocaleTimeString() + ' (auto-refreshes every 5 min)'
  } catch (err) {
    console.error('Error refreshing:', err)
    document.getElementById('lastUpdated').innerHTML = '<span class="refreshing">Refresh failed - retrying in 5 min</span>'
  }
}

// --- Event Listeners ---
document.querySelectorAll('.chain-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.chain-btn').forEach(b => b.classList.remove('active'))
    btn.classList.add('active')
    currentChain = btn.dataset.chain
    renderChain(currentChain)
  })
})

const affContainer = document.getElementById('affiliateLinks')
AFFILIATES.forEach(a => {
  const link = document.createElement('a')
  link.href = a.url
  link.target = '_blank'
  link.textContent = a.name
  link.addEventListener('click', () => {
    clicks++
    localStorage.setItem('referralClicks', clicks)
    document.getElementById('clicks').textContent = clicks
    document.getElementById('analyticsClicks').textContent = clicks
    console.log('Affiliate clicked:', a.name)
  })
  affContainer.appendChild(link)
})

document.getElementById('emailForm').addEventListener('submit', function(e) {
  e.preventDefault()
  const form = e.target
  const formData = new FormData(form)
  fetch('/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams(formData).toString()
  })
  .then(() => {
    document.getElementById('formSuccess').style.display = 'block'
    form.reset()
    setTimeout(() => { document.getElementById('formSuccess').style.display = 'none' }, 5000)
  })
  .catch(err => {
    console.error('Form error:', err)
    alert('Something went wrong. Please try again.')
  })
})

function deposit(name, url) {
  clicks++
  localStorage.setItem('referralClicks', clicks)
  document.getElementById('clicks').textContent = clicks
  document.getElementById('analyticsClicks').textContent = clicks
  console.log('Clicked to:', name)
  window.open(url, '_blank')
}

// Initialize
checkSession()
refreshData(false)
setInterval(() => refreshData(true), REFRESH_INTERVAL)
