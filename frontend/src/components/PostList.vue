<template>
  <div class="panel">
    <header style="display:flex;align-items:center;justify-content:space-between;gap:12px;">
      <div>
        <h2 style="margin:0">Posts</h2>
        <p class="status">Latest published posts</p>
      </div>
      <button @click="$emit('new')">New Post</button>
    </header>
    <div v-if="loading" class="status" style="margin-top:12px;">Loading…</div>
    <div v-else>
      <div v-if="posts.length === 0" class="status" style="margin-top:12px;">No posts yet.</div>
      <div v-else class="stack" style="margin-top:12px; display:flex; flex-direction:column; gap:10px;">
        <div v-for="p in posts" :key="p.id" class="list-item">
          <div>
            <div style="font-weight:600;">{{ p.title || '(untitled)' }}</div>
            <small>{{ p.slug }}</small>
          </div>
          <div style="display:flex; gap:8px;">
            <button class="secondary" @click="$emit('edit', p)">Edit</button>
            <button class="secondary" @click="$emit('delete', p)">Delete</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  posts: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
})

defineEmits(['new', 'edit', 'delete'])
</script>
