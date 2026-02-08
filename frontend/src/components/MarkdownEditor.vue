<template>
  <div class="markdown-editor">
    <div class="markdown-tabs">
      <button
        type="button"
        :class="['markdown-tab', activeTab === 'markdown' ? 'is-active' : '']"
        @click="activeTab = 'markdown'"
      >
        Markdown
      </button>
      <button
        type="button"
        :class="['markdown-tab', activeTab === 'preview' ? 'is-active' : '']"
        @click="activeTab = 'preview'"
      >
        Preview
      </button>
      <button
        v-if="showDiffTab"
        type="button"
        :class="['markdown-tab', activeTab === 'diff' ? 'is-active' : '']"
        @click="activeTab = 'diff'"
      >
        Diff
      </button>
    </div>

    <div v-if="activeTab === 'markdown'" class="markdown-toolbar">
      <button type="button" class="tool" title="Bold" @click="applyBold">
        <i class="ph ph-text-b"></i>
      </button>
      <button type="button" class="tool" title="Italic" @click="applyItalic">
        <i class="ph ph-text-italic"></i>
      </button>
      <button type="button" class="tool" title="Heading" @click="applyHeading">
        <i class="ph ph-text-h"></i>
      </button>
      <span class="tool-divider"></span>
      <button type="button" class="tool" title="Quote" @click="applyQuote">
        <i class="ph ph-quotes"></i>
      </button>
      <button type="button" class="tool" title="Bulleted List" @click="applyBulletList">
        <i class="ph ph-list-bullets"></i>
      </button>
      <button type="button" class="tool" title="Numbered List" @click="applyNumberedList">
        <i class="ph ph-list-numbers"></i>
      </button>
      <span class="tool-divider"></span>
      <button type="button" class="tool" title="Link" @click="applyLink">
        <i class="ph ph-link"></i>
      </button>
      <button type="button" class="tool" title="Inline Code" @click="applyInlineCode">
        <i class="ph ph-code"></i>
      </button>
      <button type="button" class="tool" title="Code Block" @click="applyCodeBlock">
        <i class="ph ph-code-block"></i>
      </button>
      <button type="button" class="tool" title="Image" @click="insertImageFromPicker">
        <i class="ph ph-image"></i>
      </button>
    </div>

    <div class="markdown-editor-body">
      <div v-show="activeTab === 'markdown'" ref="editorContainer" class="monaco-editor-root"></div>
      <div v-show="activeTab === 'preview'" class="markdown-preview markdown-body" v-html="previewHtml"></div>
      <div v-show="activeTab === 'diff'" ref="diffContainer" class="monaco-diff-root"></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import * as monaco from 'monaco-editor'
import 'monaco-editor/min/vs/editor/editor.main.css'
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import 'monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { uploadImage, isImageUploadEnabled } from '../api'

const props = defineProps({
  modelValue: { type: String, default: '' },
  diffHighlight: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue'])

const editorContainer = ref(null)
const diffContainer = ref(null)
const activeTab = ref('markdown')
const showDiffTab = computed(() => !!(props.diffHighlight && typeof props.diffHighlight.previous === 'string'))
const previewHtml = computed(() => DOMPurify.sanitize(marked.parse(props.modelValue || '')))

let editor = null
let diffEditor = null
let diffOriginalModel = null
let isApplying = false
let imageUploadEnabled = false
let pasteListener = null
let pasteEnabled = false

if (!self.MonacoEnvironment) {
  self.MonacoEnvironment = {
    getWorker: () => new editorWorker()
  }
}

async function handleImageUpload(file) {
  try {
    const result = await uploadImage(file)
    return result.url
  } catch (err) {
    alert('Failed to upload image: ' + err.message)
    return null
  }
}

function setEditorValue(value) {
  if (!editor) return
  const model = editor.getModel()
  if (!model || model.getValue() === value) return
  isApplying = true
  model.setValue(value)
  isApplying = false
}

function insertText(text) {
  if (!editor) return
  const selection = editor.getSelection()
  editor.executeEdits('markdown-toolbar', [{ range: selection, text, forceMoveMarkers: true }])
  editor.focus()
}

function wrapSelection(prefix, suffix, placeholder) {
  if (!editor) return
  const model = editor.getModel()
  const selection = editor.getSelection()
  const selected = model.getValueInRange(selection)
  const content = selected || placeholder
  const text = `${prefix}${content}${suffix}`
  editor.executeEdits('markdown-toolbar', [{ range: selection, text, forceMoveMarkers: true }])

  if (selection.startLineNumber === selection.endLineNumber) {
    const startColumn = selection.startColumn + prefix.length
    const endColumn = startColumn + content.length
    editor.setSelection(new monaco.Range(selection.startLineNumber, startColumn, selection.endLineNumber, endColumn))
  }
  editor.focus()
}

function applyLinePrefix(prefix) {
  if (!editor) return
  const model = editor.getModel()
  const selection = editor.getSelection()
  const start = selection.startLineNumber
  const end = selection.endLineNumber
  const edits = []
  for (let line = start; line <= end; line += 1) {
    edits.push({
      range: new monaco.Range(line, 1, line, 1),
      text: prefix
    })
  }
  editor.executeEdits('markdown-toolbar', edits)
  editor.focus()
}

function applyBold() {
  wrapSelection('**', '**', 'bold text')
}

function applyItalic() {
  wrapSelection('*', '*', 'italic text')
}

function applyInlineCode() {
  wrapSelection('`', '`', 'code')
}

function applyCodeBlock() {
  wrapSelection('```\n', '\n```', 'code block')
}

function applyHeading() {
  applyLinePrefix('# ')
}

function applyQuote() {
  applyLinePrefix('> ')
}

function applyBulletList() {
  applyLinePrefix('- ')
}

function applyNumberedList() {
  applyLinePrefix('1. ')
}

function applyLink() {
  wrapSelection('[', '](https://)', 'link text')
}

async function insertImageFromPicker() {
  if (!imageUploadEnabled) {
    insertText('![](https://)')
    return
  }
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.onchange = async () => {
    const file = input.files[0]
    if (!file) return
    const url = await handleImageUpload(file)
    if (url) {
      insertText(`![${file.name}](${url})`)
    }
  }
  input.click()
}

async function handlePaste(event) {
  if (!imageUploadEnabled || !event.clipboardData) return
  const files = []
  for (const item of event.clipboardData.items) {
    if (item.type && item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }
  if (files.length === 0) return
  event.preventDefault()

  for (const file of files) {
    const url = await handleImageUpload(file)
    if (url) {
      insertText(`![${file.name}](${url})\n`)
    }
  }
}

function setPasteListener(enabled) {
  if (enabled === pasteEnabled) return
  pasteEnabled = enabled
  if (enabled) {
    pasteListener = (event) => handlePaste(event)
    window.addEventListener('paste', pasteListener, true)
  } else if (pasteListener) {
    window.removeEventListener('paste', pasteListener, true)
    pasteListener = null
  }
}

function updateDiffModels(payload) {
  if (!diffEditor || !editor) return
  if (!payload || typeof payload.previous !== 'string') return

  if (!diffOriginalModel) {
    diffOriginalModel = monaco.editor.createModel(payload.previous, 'markdown')
  } else {
    diffOriginalModel.setValue(payload.previous)
  }

  diffEditor.setModel({
    original: diffOriginalModel,
    modified: editor.getModel()
  })
}

onMounted(async () => {
  try {
    imageUploadEnabled = await isImageUploadEnabled()
  } catch {
    imageUploadEnabled = false
  }

  const model = monaco.editor.createModel(props.modelValue || '', 'markdown')

  editor = monaco.editor.create(editorContainer.value, {
    model,
    language: 'markdown',
    wordWrap: 'on',
    lineNumbers: 'on',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    padding: { top: 12, bottom: 12 },
    automaticLayout: true
  })

  editor.onDidChangeModelContent(() => {
    if (!isApplying) {
      emit('update:modelValue', editor.getValue())
    }
  })

  editor.onDidFocusEditorText(() => {
    if (activeTab.value === 'markdown') {
      setPasteListener(true)
    }
  })

  editor.onDidBlurEditorText(() => {
    setPasteListener(false)
  })

  diffEditor = monaco.editor.createDiffEditor(diffContainer.value, {
    readOnly: true,
    renderSideBySide: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on',
    automaticLayout: true
  })

  updateDiffModels(props.diffHighlight)
})

watch(() => props.modelValue, (newVal) => {
  setEditorValue(newVal || '')
})

watch(() => props.diffHighlight, (payload) => {
  if (!payload || typeof payload.previous !== 'string') {
    if (activeTab.value === 'diff') {
      activeTab.value = 'markdown'
    }
    return
  }
  updateDiffModels(payload)
}, { deep: true })

watch(showDiffTab, (enabled) => {
  if (!enabled && activeTab.value === 'diff') {
    activeTab.value = 'markdown'
  }
})

watch(activeTab, async () => {
  await nextTick()
  if (activeTab.value === 'markdown' && editor) {
    editor.layout()
    editor.focus()
    setPasteListener(true)
  }
  if (activeTab.value === 'diff' && diffEditor) {
    diffEditor.layout()
  }
  if (activeTab.value !== 'markdown') {
    setPasteListener(false)
  }
})

onBeforeUnmount(() => {
  if (editor) {
    setPasteListener(false)
    editor.dispose()
    editor = null
  }
  if (diffEditor) {
    diffEditor.dispose()
    diffEditor = null
  }
  if (diffOriginalModel) {
    diffOriginalModel.dispose()
    diffOriginalModel = null
  }
})
</script>

<style>
.markdown-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
}

.markdown-tabs {
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid #e2e8f0;
  padding: 10px 12px;
  background: #f8fafc;
}

.markdown-tab {
  border: 1px solid transparent;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: #64748b;
  background: transparent;
  border-radius: 999px;
  transition: all 0.2s ease;
}

.markdown-tab:hover {
  color: #0f172a;
  background: #e2e8f0;
}

.markdown-tab.is-active {
  color: #0f172a;
  background: #fff;
  border-color: #e2e8f0;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}

.markdown-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-bottom: 1px solid #e2e8f0;
  background: #fff;
}

.markdown-toolbar .tool {
  width: 34px;
  height: 34px;
  border-radius: 8px;
  border: 1px solid transparent;
  color: #475569;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.markdown-toolbar .tool:hover {
  color: #0f172a;
  background: #e2e8f0;
}

.markdown-toolbar .tool-divider {
  width: 1px;
  height: 18px;
  background: #e2e8f0;
  margin: 0 4px;
}

.markdown-editor-body {
  flex: 1;
  min-height: 0;
  position: relative;
}

.monaco-editor-root,
.monaco-diff-root {
  height: 100%;
  width: 100%;
}

.markdown-preview {
  height: 100%;
  overflow-y: auto;
  padding: 20px 24px;
  background: #fff;
}

.monaco-editor .margin,
.monaco-editor .monaco-editor-background {
  background-color: #fff;
}

.monaco-editor .line-numbers {
  color: #cbd5e1;
}
</style>
