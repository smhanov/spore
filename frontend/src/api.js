const base = import.meta.env.BASE_URL.replace(/\/$/, '') // rely on relative paths under /blog/admin
const publicBase = base.replace(/\/admin$/, '') // public routes are at /blog, not /blog/admin

async function jsonRequest(url, options = {}) {
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Request failed ${res.status}: ${body}`)
  }
  const text = await res.text()
  return text ? JSON.parse(text) : null
}

export async function listPosts() {
  return jsonRequest(`${base}/api/posts`)
}

export async function getPost(id) {
  return jsonRequest(`${base}/api/posts/${id}`)
}

export async function createPost(data) {
  return jsonRequest(`${base}/api/posts`, { method: 'POST', body: JSON.stringify(data) })
}

export async function updatePost(id, data) {
  return jsonRequest(`${base}/api/posts/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export async function deletePost(id) {
  await jsonRequest(`${base}/api/posts/${id}`, { method: 'DELETE' })
}

// Blog settings API
export async function getBlogSettings() {
  return jsonRequest(`${base}/api/settings`)
}

export async function updateBlogSettings(data) {
  return jsonRequest(`${base}/api/settings`, { method: 'PUT', body: JSON.stringify(data) })
}

// Comment moderation API
export async function listComments(params = {}) {
  const qs = new URLSearchParams(params).toString()
  const suffix = qs ? `?${qs}` : ''
  return jsonRequest(`${base}/api/comments${suffix}`)
}

export async function updateCommentStatus(id, status) {
  await jsonRequest(`${base}/api/comments/${id}/status`, {
    method: 'PUT',
    body: JSON.stringify({ status })
  })
}

export async function deleteComment(id) {
  await jsonRequest(`${base}/api/comments/${id}`, { method: 'DELETE' })
}

export async function getNotificationConfig() {
  return jsonRequest(`${base}/api/notifications/vapid-key`)
}

export async function subscribeToNotifications(subscription) {
  await jsonRequest(`${base}/api/notifications/subscribe`, {
    method: 'POST',
    body: JSON.stringify(subscription)
  })
}

export async function unsubscribeFromNotifications(endpoint) {
  await jsonRequest(`${base}/api/notifications/subscribe`, {
    method: 'DELETE',
    body: JSON.stringify({ endpoint })
  })
}

// AI Settings API
export async function getAISettings() {
  return jsonRequest(`${base}/api/ai/settings`)
}

export async function updateAISettings(data) {
  return jsonRequest(`${base}/api/ai/settings`, { method: 'PUT', body: JSON.stringify(data) })
}

export async function sendAIChat(data) {
  return jsonRequest(`${base}/api/ai/chat`, { method: 'POST', body: JSON.stringify(data) })
}

// WXR import/export
export async function exportWXR() {
  const res = await fetch(`${base}/api/wxr/export`)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Export failed ${res.status}: ${body}`)
  }
  return res.blob()
}

export async function importWXR(file) {
  const formData = new FormData()
  formData.append('file', file)

  const res = await fetch(`${base}/api/wxr/import`, {
    method: 'POST',
    body: formData,
  })

  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Import failed ${res.status}: ${body}`)
  }

  return res.json()
}

// Image API
export async function isImageUploadEnabled() {
  const result = await jsonRequest(`${base}/api/images/enabled`)
  return result?.enabled ?? false
}

export async function uploadImage(file) {
  const formData = new FormData()
  formData.append('image', file)

  const res = await fetch(`${base}/api/images`, {
    method: 'POST',
    body: formData,
  })

  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Upload failed ${res.status}: ${body}`)
  }

  return res.json()
}

export function getImageUrl(id) {
  return `${publicBase}/images/${id}`
}
