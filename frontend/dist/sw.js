self.addEventListener('push', (event) => {
  let payload = { title: 'Spore', body: 'New activity', url: '/blog/admin?view=comments' }

  if (event.data) {
    try {
      const parsed = event.data.json()
      payload = {
        title: parsed?.title || payload.title,
        body: parsed?.body || payload.body,
        url: parsed?.url || payload.url
      }
    } catch (err) {
      const body = event.data.text()
      payload.body = body || payload.body
    }
  }

  event.waitUntil(
    self.registration.showNotification(payload.title, {
      body: payload.body,
      data: { url: payload.url }
    })
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = event.notification?.data?.url || '/blog/admin?view=comments'

  event.waitUntil((async () => {
    const clientsList = await clients.matchAll({ type: 'window', includeUncontrolled: true })
    for (const client of clientsList) {
      if ('focus' in client && client.url.includes('/blog/admin')) {
        await client.navigate(url)
        return client.focus()
      }
    }
    if (clients.openWindow) {
      return clients.openWindow(url)
    }
    return null
  })())
})
