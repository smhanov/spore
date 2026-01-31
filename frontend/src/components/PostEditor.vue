<template>
  <div class="panel">
    <div style="display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;">
      <div>
        <h2 style="margin:0">{{ model.id ? 'Edit Post' : 'New Post' }}</h2>
        <p class="status">Slug must be unique</p>
      </div>
      <div style="display:flex; gap:8px;">
        <button class="secondary" @click="$emit('cancel')">Cancel</button>
        <button @click="save">Save</button>
      </div>
    </div>

    <div class="field">
      <label>Title</label>
      <input v-model="model.title" placeholder="Post title" />
    </div>
    <div class="field">
      <label>Slug</label>
      <input v-model="model.slug" placeholder="my-post" />
    </div>
    <div class="field">
      <label>Meta Description</label>
      <textarea v-model="model.meta_description" rows="2" placeholder="SEO description"></textarea>
    </div>
    <div class="field">
      <label>Content (Markdown)</label>
      <MarkdownEditor v-model="model.content_markdown" />
    </div>
  </div>
</template>

<script setup>
import { watch, reactive } from 'vue'
import { marked } from 'marked'
import MarkdownEditor from './MarkdownEditor.vue'

const props = defineProps({
  value: { type: Object, default: () => ({}) },
})

const emit = defineEmits(['save', 'cancel', 'update:value'])

const model = reactive({
  id: props.value.id || '',
  slug: props.value.slug || '',
  title: props.value.title || '',
  content_markdown: props.value.content_markdown || '',
  content_html: props.value.content_html || '',
  meta_description: props.value.meta_description || '',
  author_id: props.value.author_id || 0,
})

watch(
  () => ({ ...props.value }),
  (v) => {
    Object.assign(model, {
      id: v.id || '',
      slug: v.slug || '',
      title: v.title || '',
      content_markdown: v.content_markdown || '',
      content_html: v.content_html || '',
      meta_description: v.meta_description || '',
      author_id: v.author_id || 0,
    })
  }
)

function save() {
  const payload = { ...model }
  // Convert markdown to HTML using marked
  payload.content_html = marked(model.content_markdown || '')
  emit('save', payload)
}
</script>
