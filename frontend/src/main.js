import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'

// Configure marked globally
import { marked } from 'marked'
marked.setOptions({
  breaks: true,
  gfm: true
})

createApp(App).mount('#app')
