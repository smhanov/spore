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
})

const emit = defineEmits(['update:modelValue'])

const textarea = ref(null)
let editor = null
let imageUploadEnabled = false

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
    emit('update:modelValue', editor.value())
  })
})

watch(() => props.modelValue, (newVal) => {
  if (editor && editor.value() !== newVal) {
    editor.value(newVal)
  }
})

onBeforeUnmount(() => {
  if (editor) {
    editor.toTextArea()
    editor = null
  }
})
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
</style>
