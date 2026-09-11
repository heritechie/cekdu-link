// @ts-nocheck
import { useState, useEffect } from 'react'

type Link = {
  id: string
  code: string
  destination_url: string
  status: 'active' | 'inactive'
  click_count: number
  short_url: string
}

type ApiLink = {
  id: string
  code: string
  destination_url: string
  status: string
  click_count: number
  short_url: string
}

const API_BASE = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

function api(path: string, options: RequestInit = {}): Promise<any> {
  return fetch(`${API_BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  }).then((res) => {
    if (!res.ok) {
      return res.json().then((err) => { throw new Error(err.error || res.statusText) })
    }
    return res.json()
  })
}

interface State {
  links: ApiLink[]
  status: 'active' | 'inactive' | null
  editingId: string | null
  creating: boolean
  toast: { message: string; type: 'success' | 'error'; autoDismiss: boolean }
}

function App() {
  const [state, setState] = useState<State>({
    links: [],
    status: null,
    editingId: null,
    creating: false,
    toast: { message: '', type: 'success', autoDismiss: true },
  })

  useEffect(() => {
    loadLinks()
  }, [])

  function loadLinks() {
    api('/api/links').then((data) => {
      setState((s) => ({ ...s, links: data }))
    }).catch((e) => {
      showToast('Unable to load links.', true)
    })
  }

  function showToast(m: string, t = 'error') {
    setState((s) => ({ ...s, toast: { message: m, type: t, autoDismiss: true } }))
    setTimeout(() => setState((s) => ({ ...s, toast: { message: '', type: 'success', autoDismiss: true } })), 3000)
  }

  function toggleStatus(id: string) {
    const link = state.links.find((l) => l.id === id)
    if (!link) return
    const target = link.status === 'active' ? 'inactive' : 'active'
    api(`/api/links/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ status: target }),
    }).then(() => {
      showToast(target === 'inactive' ? 'Link disabled' : 'Link activated')
      loadLinks()
    }).catch((e) => showToast(e.message, true))
  }

  function startEdit(id: string) {
    setState((s) => ({ ...s, editingId: id }))
  }

  function closeEdit() {
    setState((s) => ({ ...s, editingId: null }))
  }

  function saveEdit(id: string, dest: string, status: 'active' | 'inactive') {
    api(`/api/links/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ destination_url: dest, status }),
    }).then(() => {
      showToast('Link updated')
      closeEdit()
      loadLinks()
    }).catch((e) => showToast(e.message, true))
  }

  function createLink(e: Event) {
    e.preventDefault()
    const dest = (document.getElementById('create-dest') as HTMLInputElement).value.trim()
    const code = (document.getElementById('create-code') as HTMLInputElement).value.trim()
    if (!dest) return
    api('/api/links', {
      method: 'POST',
      body: JSON.stringify({ destination_url: dest, code }),
    }).then((link) => {
      showToast('Link created')
      loadLinks()
      ;(document.getElementById('create-dest') as HTMLInputElement).value = ''
      ;(document.getElementById('create-code') as HTMLInputElement).value = ''
    }).catch((e) => showToast(e.message, true))
  }

  var copyTimer: NodeJS.Timeout | null = null

  function copyText(text: string) {
    clearTimeout(copyTimer)
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        showToast('Copied to clipboard')
      }).catch(() => {
        const ta = document.createElement('textarea')
        ta.value = text
        ta.style.position = 'fixed'
        ta.style.opacity = '0'
        document.body.appendChild(ta)
        ta.focus()
        ta.select()
        try {
          document.execCommand('copy')
          showToast('Copied to clipboard')
        } catch (e) {
          showToast('Copy failed. Select and copy: ' + text, true)
        }
        ta.remove()
      })
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.focus()
      ta.select()
      try {
        document.execCommand('copy')
        showToast('Copied to clipboard')
      } catch (e) {
        showToast('Copy failed. Select and copy: ' + text, true)
      }
      ta.remove()
    }
  }

  function renderLinks(): any {
    if (state.links.length === 0) {
      return createElement('p', null, 'No short links yet.<br/>Create your first short link.')
    }
    const cells = state.links.map((link) =>
      createElement('div', { style: { border: '1px solid #ddd', padding: '12px', marginBottom: '12px' } },
        createElement('div', null,
          'Code: ', link.code,
          '<br/>',
          'Destination: ', link.destination_url,
          '<br/>',
          'Clicks: ', link.click_count,
          '<br/>',
          'Status: ', link.status,
          '<br/>',
          'Short URL: ', link.short_url,
          '<br/>',
          <button onClick={() => toggleStatus(link.id)} className="btn">
            {link.status === 'active' ? 'Disable' : 'Activate'}
          </button>
        )
      )
    )
    return createElement('div', null, cells)
  }

  return createElement('div', null,
    createElement('div', { className: 'mb-6' },
      createElement('h1', { className: 'text-2xl font-bold' }, 'cekdu-link'),
      <button onClick={() => setState((s) => ({ ...s, creating: !s.creating }))} className="btn">
        {state.creating ? 'Cancel' : '+ Create Link'}
      </button>
    ),
    <div className="mt-6">
      {state.toast.message && createElement('div', null, state.toast.message)}
      {state.editingId && (
        <div>
          <h2>Edit Link <span>{state.links.find((l) => l.id === state.editingId)?.code ?? ''}</span></h2>
          <form onSubmit={(e) => {
            e.preventDefault()
            const dest = (document.getElementById('edit-dest') as HTMLInputElement).value.trim()
            const status = (document.getElementById('edit-status') as HTMLSelectElement).value as 'active' | 'inactive'
            saveEdit(state.editingId, dest, status)
          }}>
            <input
              id="edit-dest"
              type="url"
              required
              className="input input-bordered w-full"
              value={(document.getElementById('edit-dest') as HTMLInputElement)?.value ?? ''}
            />
            <select id="edit-status" className="input input-bordered w-full">
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
            </select>
            <div className="flex justify-end">
              <button onClick={closeEdit} className="btn">Cancel</button>
              <button className="btn btn-primary">Save</button>
            </div>
          </form>
        )}

        {state.links.length === 0 && (
          <p>No short links yet.<br/>Create your first short link.</p>
        )}

        {state.links.length > 0 && (
          <div>
            {renderLinks()}
          </div>
        )}
      </div>
    </div>
  )
}

type State = {
  links: ApiLink[]
  status: 'active' | 'inactive' | null
  editingId: string | null
  creating: boolean
  toast: { message: string; type: 'success' | 'error'; autoDismiss: boolean }
}

export default App
