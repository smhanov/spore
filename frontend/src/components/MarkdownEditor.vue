<template>
  <div class="markdown-editor-container">
    <textarea ref="textarea"></textarea>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import EasyMDE from 'easymde'
import 'easymde/dist/easymde.min.css'
import { uploadImage, isImageUploadEnabled } from '../api'

const props = defineProps({
  modelValue: { type: String, default: '' },
  diffHighlight: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue'])

const textarea = ref(null)
let editor = null
let imageUploadEnabled = false
let highlightedLines = []
let diffWidgets = []
let baselineLines = null
let isApplying = false

async function handleImageUpload(file) {
  try {
    const result = await uploadImage(file)
    return result.url
  } catch (err) {
    alert('Failed to upload image: ' + err.message)
    return null
  }
}

onMounted(async () => {
  // Check if image upload is enabled
  try {
    imageUploadEnabled = await isImageUploadEnabled()
  } catch {
    imageUploadEnabled = false
  }

  const toolbar = [
    'bold', 'italic', 'heading', '|',
    'quote', 'unordered-list', 'ordered-list', '|',
    'link',
    ...(imageUploadEnabled ? ['image', {
      name: 'upload-image',
      action: async (editor) => {
        const input = document.createElement('input')
        input.type = 'file'
        input.accept = 'image/*'
        input.onchange = async () => {
          const file = input.files[0]
          if (!file) return
          
          const url = await handleImageUpload(file)
          if (url) {
            const cm = editor.codemirror
            const pos = cm.getCursor()
            cm.replaceRange(`![${file.name}](${url})`, pos)
          }
        }
        input.click()
      },
      className: 'fa fa-upload',
      title: 'Upload Image',
    }] : ['image']),
    '|',
    'preview', 'side-by-side', 'fullscreen', '|',
    'guide'
  ]

  editor = new EasyMDE({
    element: textarea.value,
    initialValue: props.modelValue,
    spellChecker: false,
    autofocus: false,
    placeholder: 'Write your content using Markdown... (paste or drag images here)',
    toolbar,
    uploadImage: imageUploadEnabled,
    imageUploadFunction: imageUploadEnabled ? async (file, onSuccess, onError) => {
      try {
        const result = await uploadImage(file)
        onSuccess(result.url)
      } catch (err) {
        onError(err.message)
      }
    } : undefined,
    imageAccept: 'image/png, image/jpeg, image/gif, image/webp',
    status: ['lines', 'words', 'cursor'],
    renderingConfig: {
      singleLineBreaks: false,
      codeSyntaxHighlighting: true,
    },
  })

  editor.codemirror.on('change', () => {
    if (!isApplying) {
      if (highlightedLines.length > 0) {
        clearHighlights()
      }
      if (diffWidgets.length > 0) {
        clearWidgets()
      }
      baselineLines = null
    }
    emit('update:modelValue', editor.value())
  })
})

watch(() => props.modelValue, (newVal) => {
  if (editor && editor.value() !== newVal) {
    editor.value(newVal)
  }
})

watch(() => props.diffHighlight, (payload) => {
  if (!editor) return
  if (!payload || typeof payload.previous !== 'string' || typeof payload.current !== 'string') {
    clearHighlights()
    clearWidgets()
    baselineLines = null
    return
  }
  applyHighlights(payload.previous, payload.current)
}, { deep: true })

onBeforeUnmount(() => {
  if (editor) {
    editor.toTextArea()
    editor = null
  }
})

function clearHighlights() {
  if (!editor || highlightedLines.length === 0) return
  const cm = editor.codemirror
  highlightedLines.forEach((line) => {
    cm.removeLineClass(line, 'background', 'ai-diff-add')
  })
  highlightedLines = []
}

function clearWidgets() {
  if (diffWidgets.length === 0) return
  diffWidgets.forEach((widget) => widget.clear())
  diffWidgets = []
}

function applyHighlights(previous, current) {
  if (!editor) return
  clearHighlights()
  clearWidgets()
  baselineLines = previous.split('\n')
  renderInlineDiff(current)
}

function renderInlineDiff(current) {
  if (!editor || !baselineLines) return
  clearHighlights()
  clearWidgets()

  const currentLines = current.split('\n')
  const diff = computeLineDiff(baselineLines, currentLines)
  const cm = editor.codemirror

  diff.added.forEach((change) => {
    const line = Math.min(change.index, Math.max(cm.lineCount() - 1, 0))
    cm.addLineClass(line, 'background', 'ai-diff-add')
    const widget = cm.addLineWidget(line, createChangeWidget(change, 'added'), { above: false })
    diffWidgets.push(widget)
  })
  highlightedLines = diff.added.map((change) => Math.min(change.index, Math.max(cm.lineCount() - 1, 0)))

  diff.removed.forEach((change) => {
    const line = Math.min(change.index, Math.max(cm.lineCount() - 1, 0))
    const widget = cm.addLineWidget(line, createChangeWidget(change, 'removed'), { above: true })
    diffWidgets.push(widget)
  })
}

function computeLineDiff(oldLines, newLines) {
  const rows = oldLines.length
  const cols = newLines.length
  const dp = Array.from({ length: rows + 1 }, () => Array(cols + 1).fill(0))

  for (let i = rows - 1; i >= 0; i -= 1) {
    for (let j = cols - 1; j >= 0; j -= 1) {
      if (oldLines[i] === newLines[j]) {
        dp[i][j] = dp[i + 1][j + 1] + 1
      } else {
        dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1])
      }
    }
  }

  const added = []
  const removed = []
  let i = 0
  let j = 0
  while (i < rows && j < cols) {
    if (oldLines[i] === newLines[j]) {
      i += 1
      j += 1
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      removed.push({ type: 'remove', index: j, text: oldLines[i] })
      i += 1
    } else {
      added.push({ type: 'add', index: j, text: newLines[j] })
      j += 1
    }
  }
  while (i < rows) {
    removed.push({ type: 'remove', index: j, text: oldLines[i] })
    i += 1
  }
  while (j < cols) {
    added.push({ type: 'add', index: j, text: newLines[j] })
    j += 1
  }

  return { added, removed }
}

function createChangeWidget(change, kind) {
  const node = document.createElement('div')
  node.className = `ai-diff-widget ai-diff-${kind}`

  const text = document.createElement('div')
  text.className = 'ai-diff-widget__text'
  text.textContent = kind === 'removed' ? `- ${change.text}` : `+ ${change.text}`

  const actions = document.createElement('div')
  actions.className = 'ai-diff-widget__actions'

  const acceptBtn = document.createElement('button')
  acceptBtn.type = 'button'
  acceptBtn.className = 'ai-diff-btn'
  acceptBtn.textContent = 'Accept'
  acceptBtn.onclick = (event) => {
    event.preventDefault()
    acceptChange(change)
  }

  const undoBtn = document.createElement('button')
  undoBtn.type = 'button'
  undoBtn.className = 'ai-diff-btn ai-diff-btn--muted'
  undoBtn.textContent = 'Undo'
  undoBtn.onclick = (event) => {
    event.preventDefault()
    undoChange(change)
  }

  actions.appendChild(acceptBtn)
  actions.appendChild(undoBtn)
  node.appendChild(text)
  node.appendChild(actions)

  return node
}

function acceptChange(change) {
  if (!baselineLines || !editor) return
  if (change.type === 'add') {
    baselineLines.splice(change.index, 0, change.text)
  } else {
    if (baselineLines[change.index] === change.text) {
      baselineLines.splice(change.index, 1)
    } else {
      const idx = baselineLines.indexOf(change.text)
      if (idx > -1) {
        baselineLines.splice(idx, 1)
      }
    }
  }
  renderInlineDiff(editor.value())
}

function undoChange(change) {
  if (!editor || !baselineLines) return
  const currentLines = editor.value().split('\n')

  if (change.type === 'add') {
    if (currentLines[change.index] === change.text) {
      currentLines.splice(change.index, 1)
    } else {
      const idx = currentLines.indexOf(change.text)
      if (idx > -1) {
        currentLines.splice(idx, 1)
      }
    }
  } else {
    currentLines.splice(change.index, 0, change.text)
  }

  setEditorValue(currentLines.join('\n'))
  renderInlineDiff(editor.value())
}

function setEditorValue(value) {
  if (!editor) return
  isApplying = true
  editor.value(value)
  isApplying = false
}
</script>

<style>
.markdown-editor-container {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.markdown-editor-container .EasyMDEContainer {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.markdown-editor-container .CodeMirror {
  flex: 1;
  min-height: 300px;
  border: none;
  border-radius: 0;
  font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
  font-size: 14px;
  line-height: 1.6;
  color: #334155;
  background: #fff;
}

.markdown-editor-container .CodeMirror-scroll {
  min-height: 300px;
}

.markdown-editor-container .editor-toolbar {
  border: none;
  border-bottom: 1px solid #e2e8f0;
  border-radius: 0;
  background: #f8fafc;
  padding: 8px 12px;
}

.markdown-editor-container .editor-toolbar button {
  color: #64748b !important;
  border: none !important;
  border-radius: 6px !important;
  width: 32px;
  height: 32px;
}

.markdown-editor-container .editor-toolbar button:hover {
  background: #e2e8f0 !important;
  color: #1e293b !important;
}

.markdown-editor-container .editor-toolbar button.active {
  background: #3b82f6 !important;
  color: white !important;
}

.markdown-editor-container .editor-toolbar i.separator {
  border-left-color: #e2e8f0;
}

.markdown-editor-container .editor-preview {
  background: #f8fafc;
  padding: 16px;
}

.markdown-editor-container .editor-preview-side {
  border-left: 1px solid #e2e8f0;
  background: #f8fafc;
}

.markdown-editor-container .editor-statusbar {
  border-top: 1px solid #e2e8f0;
  padding: 8px 12px;
  color: #94a3b8;
  font-size: 12px;
}

/* Fix for fullscreen and side-by-side mode */
.markdown-editor-container .EasyMDEContainer.fullscreen,
.markdown-editor-container .EasyMDEContainer.sidesided {
  z-index: 9999;
  position: fixed !important;
  top: 0 !important;
  left: 0 !important;
  right: 0 !important;
  bottom: 0 !important;
  width: 100% !important;
  height: 100% !important;
}

/* Drag and drop indicator */
.markdown-editor-container .CodeMirror-dragover {
  background: #eff6ff;
  border: 2px dashed #3b82f6;
}

.markdown-editor-container .CodeMirror .ai-diff-add {
  background: #ecfdf3;
}

.markdown-editor-container .ai-diff-widget {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 8px;
  font-size: 12px;
  line-height: 1.4;
  font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
  border-left: 2px solid transparent;
  border-radius: 6px;
  margin: 2px 0;
}

.markdown-editor-container .ai-diff-widget__text {
  flex: 1;
  white-space: pre-wrap;
}

.markdown-editor-container .ai-diff-widget__actions {
  display: flex;
  gap: 6px;
}

.markdown-editor-container .ai-diff-btn {
  border: 1px solid #cbd5f5;
  background: #f8fafc;
  color: #1f2937;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 6px;
  cursor: pointer;
}

.markdown-editor-container .ai-diff-btn--muted {
  border-color: #e2e8f0;
  color: #6b7280;
}

.markdown-editor-container .ai-diff-added {
  background: #ecfdf3;
  border-left-color: #10b981;
  color: #065f46;
}

.markdown-editor-container .ai-diff-removed {
  background: #fef2f2;
  border-left-color: #ef4444;
  color: #991b1b;
  text-decoration: line-through;
}
</style>
