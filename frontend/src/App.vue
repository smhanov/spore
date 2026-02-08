<template>
  <div class="bg-surface text-slate-800 antialiased h-screen overflow-hidden selection:bg-brand-100 selection:text-brand-900">
    <div class="h-full flex flex-col md:flex-row relative">
      
      <!-- Mobile Header -->
      <header class="md:hidden flex items-center justify-between p-4 glass-panel sticky top-0 z-20">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-brand-600 flex items-center justify-center text-white">
            <i class="ph ph-hexagon text-xl"></i>
          </div>
          <span class="font-bold text-lg tracking-tight">Blog Admin</span>
        </div>
        <button @click="sidebarOpen = !sidebarOpen" class="p-2 text-slate-600 hover:bg-slate-100 rounded-full transition-colors">
          <i class="ph ph-list text-2xl"></i>
        </button>
      </header>

      <!-- Sidebar Navigation -->
      <aside :class="[
        'fixed md:static inset-y-0 left-0 z-30 w-64 bg-white/80 md:bg-white/50 backdrop-blur-xl border-r border-slate-200/60 transform transition-transform duration-300 ease-in-out flex flex-col',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
      ]">
        <div class="p-6 hidden md:flex items-center gap-3 mb-6">
          <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-brand-500 to-indigo-600 flex items-center justify-center text-white shadow-lg shadow-brand-500/30">
            <i class="ph ph-hexagon-fill text-2xl"></i>
          </div>
          <div>
            <h1 class="font-bold text-xl tracking-tight text-slate-900">Blog</h1>
            <p class="text-xs text-slate-500 font-medium">Admin Panel</p>
          </div>
        </div>

        <nav class="flex-1 px-4 space-y-1 overflow-y-auto">
          <p class="px-2 text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2 mt-4">Content</p>
          
          <a href="#" @click.prevent="currentView = 'list'; sidebarOpen = false" 
             :class="['flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 group', 
             currentView === 'list' ? 'bg-brand-50 text-brand-700 shadow-sm ring-1 ring-brand-100' : 'text-slate-600 hover:bg-white hover:shadow-sm']">
            <i :class="['ph text-lg', currentView === 'list' ? 'ph-article-ny-times-fill' : 'ph-article-ny-times']"></i>
            All Posts
            <span class="ml-auto text-xs font-bold px-2 py-0.5 rounded-full" 
                  :class="currentView === 'list' ? 'bg-brand-100 text-brand-700' : 'bg-slate-100 text-slate-500'">
              {{ posts.length }}
            </span>
          </a>

          <a href="#" @click.prevent="currentView = 'ai-settings'; sidebarOpen = false" 
             :class="['flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 group', 
             currentView === 'ai-settings' ? 'bg-brand-50 text-brand-700 shadow-sm ring-1 ring-brand-100' : 'text-slate-600 hover:bg-white hover:shadow-sm']">
            <i :class="['ph text-lg', currentView === 'ai-settings' ? 'ph-brain-fill' : 'ph-brain']"></i>
            AI Settings
          </a>
        </nav>

        <div class="p-4 border-t border-slate-200/60">
          <div class="flex items-center gap-3 w-full p-2 rounded-xl">
            <div class="w-8 h-8 rounded-full bg-brand-100 flex items-center justify-center text-brand-600">
              <i class="ph ph-user text-lg"></i>
            </div>
            <div class="text-left flex-1 min-w-0">
              <p class="text-sm font-semibold text-slate-900 truncate">Admin</p>
              <p class="text-xs text-slate-500 truncate">Blog Administrator</p>
            </div>
          </div>
        </div>
      </aside>

      <!-- Overlay for mobile sidebar -->
      <div v-if="sidebarOpen" @click="sidebarOpen = false" class="fixed inset-0 bg-slate-900/20 backdrop-blur-sm z-20 md:hidden"></div>

      <!-- Main Content Area -->
      <main class="flex-1 overflow-hidden relative flex flex-col bg-slate-50/50">
        
        <!-- Toast Notification -->
        <div v-if="toast.show" class="fixed top-4 right-4 z-50 toast">
          <div :class="['px-4 py-3 rounded-xl shadow-lg flex items-center gap-3', 
                        toast.type === 'success' ? 'bg-emerald-500 text-white' : 'bg-rose-500 text-white']">
            <i :class="['ph text-lg', toast.type === 'success' ? 'ph-check-circle' : 'ph-warning-circle']"></i>
            <span class="text-sm font-medium">{{ toast.message }}</span>
          </div>
        </div>

        <!-- VIEW: POST LIST -->
        <div v-if="currentView === 'list'" class="h-full flex flex-col">
          <!-- Top Toolbar -->
          <div class="p-4 md:p-8 pb-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-2xl font-bold text-slate-900">Posts</h2>
              <p class="text-slate-500 text-sm mt-1">Manage and publish your blog content</p>
            </div>
            <button @click="createNewPost" class="bg-slate-900 hover:bg-slate-800 text-white px-5 py-2.5 rounded-xl text-sm font-medium shadow-lg shadow-slate-900/20 flex items-center gap-2 transition-all active:scale-95 w-full sm:w-auto justify-center">
              <i class="ph ph-plus-bold"></i> New Post
            </button>
          </div>

          <!-- Filters & Search -->
          <div class="px-4 md:px-8 pb-6">
            <div class="flex flex-col sm:flex-row gap-3">
              <div class="relative flex-1">
                <i class="ph ph-magnifying-glass absolute left-3 top-1/2 -translate-y-1/2 text-slate-400"></i>
                <input v-model="searchQuery" type="text" placeholder="Search posts..." class="w-full pl-10 pr-4 py-2.5 bg-white border border-slate-200 rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all shadow-sm">
              </div>
              <div class="flex gap-2 overflow-x-auto pb-1 sm:pb-0">
                <button @click="filterStatus = 'all'" 
                    :class="['px-4 py-2.5 rounded-xl text-sm font-medium whitespace-nowrap transition-all border', filterStatus === 'all' ? 'bg-white border-slate-200 text-slate-900 shadow-sm' : 'border-transparent text-slate-500 hover:bg-white hover:text-slate-700']">
                  All Posts
                </button>
                <button @click="filterStatus = 'published'"
                    :class="['px-4 py-2.5 rounded-xl text-sm font-medium whitespace-nowrap transition-all border', filterStatus === 'published' ? 'bg-emerald-50 border-emerald-100 text-emerald-700 shadow-sm' : 'border-transparent text-slate-500 hover:bg-white hover:text-slate-700']">
                  Published
                </button>
                <button @click="filterStatus = 'draft'"
                    :class="['px-4 py-2.5 rounded-xl text-sm font-medium whitespace-nowrap transition-all border', filterStatus === 'draft' ? 'bg-amber-50 border-amber-100 text-amber-700 shadow-sm' : 'border-transparent text-slate-500 hover:bg-white hover:text-slate-700']">
                  Drafts
                </button>
              </div>
            </div>
          </div>

          <!-- Loading State -->
          <div v-if="loading" class="flex-1 flex items-center justify-center">
            <div class="flex flex-col items-center gap-3 text-slate-400">
              <i class="ph ph-spinner text-4xl animate-spin"></i>
              <p class="text-sm">Loading posts...</p>
            </div>
          </div>

          <!-- List Content -->
          <div v-else class="flex-1 overflow-y-auto px-4 md:px-8 pb-8">
            <div v-if="filteredPosts.length === 0" class="flex flex-col items-center justify-center h-64 text-slate-400">
              <i class="ph ph-files text-4xl mb-3 opacity-50"></i>
              <p>No posts found matching your criteria.</p>
            </div>

            <transition-group name="list" tag="div" class="space-y-3">
              <div v-for="post in filteredPosts" :key="post.id" 
                   class="group bg-white rounded-2xl p-4 border border-slate-200/60 shadow-sm hover:shadow-md hover:border-brand-200 transition-all duration-300 relative overflow-hidden">
                
                <!-- Card Body -->
                <div class="flex flex-col md:flex-row gap-4 items-start md:items-center">
                  <!-- Status Indicator -->
                  <div class="w-12 h-12 rounded-xl flex items-center justify-center shrink-0"
                       :class="post.published_at ? 'bg-emerald-50 text-emerald-600' : 'bg-amber-50 text-amber-600'">
                    <i :class="['text-xl ph', post.published_at ? 'ph-check-circle-fill' : 'ph-pencil-simple-slash-fill']"></i>
                  </div>

                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 mb-1">
                      <h3 class="font-bold text-slate-900 truncate text-lg group-hover:text-brand-600 transition-colors">{{ post.title || '(Untitled)' }}</h3>
                    </div>
                    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-slate-500 font-medium">
                      <span class="flex items-center gap-1">
                        <i class="ph ph-calendar-blank"></i> {{ formatDate(post.published_at) }}
                      </span>
                      <span class="flex items-center gap-1 font-mono text-slate-400 bg-slate-50 px-1.5 py-0.5 rounded">
                        /{{ post.slug }}
                      </span>
                    </div>
                  </div>

                  <!-- Actions -->
                  <div class="flex items-center gap-2 w-full md:w-auto mt-2 md:mt-0 pt-3 md:pt-0 border-t md:border-t-0 border-slate-100">
                    <a v-if="post.published_at" :href="`/blog/${post.slug}`" target="_blank" class="p-2 rounded-lg text-slate-400 hover:bg-sky-50 hover:text-sky-600 transition-colors" title="View Public Post">
                      <i class="ph ph-arrow-square-out text-lg"></i>
                    </a>
                    <button @click="editPost(post)" class="flex-1 md:flex-none py-2 md:py-1.5 px-4 rounded-lg bg-slate-50 text-slate-600 hover:bg-brand-50 hover:text-brand-600 text-sm font-medium transition-colors">
                      Edit
                    </button>
                    <button @click="confirmDeletePost(post)" class="p-2 rounded-lg text-slate-400 hover:bg-rose-50 hover:text-rose-600 transition-colors">
                      <i class="ph ph-trash text-lg"></i>
                    </button>
                  </div>
                </div>
              </div>
            </transition-group>
          </div>
        </div>

        <!-- VIEW: POST EDITOR -->
        <div v-else-if="currentView === 'editor'" class="h-full flex flex-col bg-white">
          
          <!-- Editor Toolbar -->
          <div class="h-16 border-b border-slate-200 flex items-center justify-between px-4 md:px-6 bg-white z-10">
            <div class="flex items-center gap-3">
              <button @click="confirmBack" class="p-2 -ml-2 text-slate-400 hover:text-slate-800 hover:bg-slate-100 rounded-full transition-colors">
                <i class="ph ph-arrow-left text-xl"></i>
              </button>
              <div class="h-6 w-px bg-slate-200 hidden md:block"></div>
              <span class="text-sm font-semibold text-slate-500 hidden md:block">
                {{ isNewPost ? 'New Post' : 'Editing Post' }}
              </span>
            </div>

            <div class="flex items-center gap-3">
              <div class="flex items-center gap-2 mr-2">
                <span class="text-xs font-semibold text-slate-500 uppercase hidden sm:block">Published</span>
                <button @click="draftPost.published = !draftPost.published" 
                    :class="['w-11 h-6 rounded-full relative transition-colors duration-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-brand-500', draftPost.published ? 'bg-emerald-500' : 'bg-slate-300']">
                  <span :class="['absolute top-1 left-1 w-4 h-4 bg-white rounded-full shadow-sm transition-transform duration-300', draftPost.published ? 'translate-x-5' : 'translate-x-0']"></span>
                </button>
              </div>
              <button @click="savePost" :disabled="saving" 
                  :class="['text-white px-5 py-2 rounded-xl text-sm font-bold shadow-lg transition-all active:scale-95 flex items-center gap-2', 
                  saving ? 'bg-slate-800 opacity-50 cursor-not-allowed' : 
                  hasUnsavedChanges ? 'bg-amber-600 hover:bg-amber-700 shadow-amber-600/20' : 'bg-slate-900 hover:bg-slate-800 shadow-slate-900/20']">
                <i v-if="hasUnsavedChanges && !saving" class="ph ph-warning-circle text-lg"></i>
                <i v-if="saving" class="ph ph-spinner animate-spin text-lg"></i>
                {{ saving ? 'Saving...' : (hasUnsavedChanges ? 'Unsaved Changes' : 'Saved') }}
              </button>
            </div>
          </div>

          <div class="flex-1 flex overflow-hidden">
            
            <!-- Editor Main Area -->
            <div class="flex-1 flex flex-col h-full overflow-hidden relative">
              <!-- Meta Fields -->
              <div class="border-b border-slate-100 p-4 md:px-8 md:pt-8 md:pb-4 space-y-4 bg-white">
                <input v-model="draftPost.title" @input="autoSlug" type="text" placeholder="Post Title" 
                       class="w-full text-2xl md:text-3xl font-bold placeholder-slate-300 border-none focus:ring-0 p-0 text-slate-900 bg-transparent outline-none">
                
                <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div class="flex items-center gap-2 text-slate-500 border-b border-slate-100 pb-1 focus-within:border-brand-500 transition-colors">
                    <i class="ph ph-link text-lg"></i>
                    <span class="text-xs font-mono text-slate-400">/</span>
                    <input v-model="draftPost.slug" type="text" placeholder="url-slug" class="w-full text-sm font-mono bg-transparent border-none focus:ring-0 p-0 text-slate-600 outline-none">
                  </div>
                  
                  <!-- Date Input -->
                  <div class="flex items-center gap-2 text-slate-500 border-b border-slate-100 pb-1 focus-within:border-brand-500 transition-colors">
                    <i class="ph ph-calendar-blank text-lg"></i>
                    <input v-model="draftPost.date" type="date" class="w-full text-sm bg-transparent border-none focus:ring-0 p-0 text-slate-600 placeholder-slate-400 font-sans outline-none">
                  </div>
                </div>
              </div>

              <!-- Split View Container -->
              <div class="flex-1 flex overflow-hidden">
                <!-- Markdown Editor with EasyMDE -->
                <div class="flex-1 flex flex-col h-full relative">
                  <MarkdownEditor v-model="draftPost.content" :diffHighlight="aiHighlight" />
                </div>
              </div>
            </div>

            <!-- Right Sidebar (Settings) - Desktop Only -->
            <div class="w-72 border-l border-slate-200 bg-white hidden lg:flex flex-col overflow-y-auto">
              <div class="p-4 border-b border-slate-100">
                <h3 class="font-bold text-sm text-slate-900">SEO & Settings</h3>
              </div>
              
              <div class="p-4 space-y-6">
                <!-- SEO Description -->
                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">
                    Meta Description
                    <span :class="['ml-1', seoLengthColor]">{{ draftPost.description.length }}/160</span>
                  </label>
                  <textarea v-model="draftPost.description" rows="4" 
                      class="w-full text-sm p-3 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none transition-all resize-none text-slate-600 leading-relaxed"
                      placeholder="Enter a meta description..."></textarea>
                </div>

                <!-- Tags Display -->
                <div v-if="draftPost.tags && draftPost.tags.length > 0" class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Tags</label>
                  <div class="flex flex-wrap gap-2">
                    <span v-for="tag in draftPost.tags" :key="tag.id" class="px-2 py-1 bg-brand-50 text-brand-600 rounded-md text-xs font-bold">
                      {{ tag.name }}
                    </span>
                  </div>
                </div>

                <!-- AI Assistant -->
                <div class="space-y-3 border-t border-slate-100 pt-4">
                  <div class="flex items-center justify-between">
                    <label class="text-xs font-semibold text-slate-500 uppercase">AI Assistant</label>
                    <span v-if="aiEnabled.smart || aiEnabled.dumb" class="text-[11px] text-emerald-600 font-semibold">Ready</span>
                    <span v-else class="text-[11px] text-slate-400 font-semibold">Not configured</span>
                  </div>

                  <div v-if="aiEnabled.smart || aiEnabled.dumb" class="space-y-3">
                    <div class="flex items-center gap-2">
                      <button @click="aiMode = 'smart'" :class="['px-2 py-1 rounded-md text-xs font-semibold border', aiMode === 'smart' ? 'bg-brand-50 text-brand-700 border-brand-100' : 'border-slate-200 text-slate-500']">Smart</button>
                      <button @click="aiMode = 'dumb'" :disabled="!aiEnabled.dumb" :class="['px-2 py-1 rounded-md text-xs font-semibold border', aiMode === 'dumb' ? 'bg-brand-50 text-brand-700 border-brand-100' : 'border-slate-200 text-slate-500', !aiEnabled.dumb ? 'opacity-50 cursor-not-allowed' : '']">Dumb</button>
                    </div>

                    <label class="flex items-center gap-2 text-[11px] text-slate-500">
                      <input v-model="aiUseSearch" type="checkbox" class="accent-brand-600">
                      Use web search if supported
                    </label>

                    <label class="flex items-center gap-2 text-[11px] text-slate-500">
                      <input v-model="aiHighlightEnabled" type="checkbox" class="accent-brand-600">
                      Highlight applied changes in editor
                    </label>

                    <textarea v-model="aiQuery" rows="3" placeholder="Ask for edits, rewrites, or tone changes..." class="w-full text-xs p-2 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none resize-none"></textarea>
                    <button @click="sendAI" :disabled="aiBusy || !aiQuery" :class="['w-full text-xs font-semibold px-3 py-2 rounded-lg text-white transition-all', aiBusy ? 'bg-slate-400 cursor-not-allowed' : 'bg-slate-900 hover:bg-slate-800']">
                      {{ aiBusy ? 'Thinking...' : 'Send to AI' }}
                    </button>

                    <div v-if="aiNotes" class="text-xs text-slate-600 bg-slate-50 border border-slate-100 rounded-lg p-2">
                      <span class="font-semibold text-slate-700">Notes:</span> {{ aiNotes }}
                    </div>

                    <p class="text-[11px] text-slate-500">Changes apply instantly. Use inline Accept/Undo buttons inside the editor.</p>
                  </div>

                  <div v-else class="text-xs text-slate-500">Configure AI providers in the settings page to enable the assistant.</div>
                </div>
              </div>
            </div>

          </div>
        </div>

        <!-- VIEW: AI SETTINGS -->
        <div v-else-if="currentView === 'ai-settings'" class="h-full flex flex-col overflow-y-auto">
          <div class="p-4 md:p-8 pb-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-2xl font-bold text-slate-900">AI Settings</h2>
              <p class="text-slate-500 text-sm mt-1">Configure smart and dumb models for the editor assistant</p>
            </div>
            <button @click="saveAISettings" :disabled="aiSaving" 
              :class="['text-white px-5 py-2.5 rounded-xl text-sm font-medium shadow-lg transition-all active:scale-95 flex items-center gap-2', aiSaving ? 'bg-slate-500 cursor-not-allowed' : 'bg-slate-900 hover:bg-slate-800']">
              <i v-if="aiSaving" class="ph ph-spinner animate-spin"></i>
              {{ aiSaving ? 'Saving...' : 'Save Settings' }}
            </button>
          </div>

          <div class="px-4 md:px-8 pb-8 space-y-6">
            <div v-if="aiLoading" class="flex items-center gap-2 text-slate-500">
              <i class="ph ph-spinner animate-spin"></i>
              Loading settings...
            </div>

            <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <div class="bg-white border border-slate-200/60 rounded-2xl p-5 shadow-sm space-y-4">
                <div class="flex items-center justify-between">
                  <h3 class="text-lg font-bold text-slate-900">Smart AI</h3>
                  <span :class="['text-xs font-semibold px-2 py-1 rounded-full', aiEnabled.smart ? 'bg-emerald-50 text-emerald-700' : 'bg-slate-100 text-slate-500']">
                    {{ aiEnabled.smart ? 'Enabled' : 'Disabled' }}
                  </span>
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Provider</label>
                  <select v-model="aiSettings.smart.provider" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                    <option value="">Select provider</option>
                    <option value="openai">OpenAI</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="gemini">Gemini</option>
                    <option value="ollama">Ollama</option>
                  </select>
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Model</label>
                  <input v-model="aiSettings.smart.model" type="text" placeholder="gpt-4o-mini" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">API Key</label>
                  <input v-model="aiSettings.smart.api_key" type="password" placeholder="sk-..." class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Base URL</label>
                  <input v-model="aiSettings.smart.base_url" type="text" placeholder="https://api.openai.com" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div class="space-y-2">
                    <label class="text-xs font-semibold text-slate-500 uppercase">Temperature</label>
                    <input v-model.number="aiSettings.smart.temperature" type="number" step="0.1" min="0" max="2" placeholder="0.4" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                  </div>
                  <div class="space-y-2">
                    <label class="text-xs font-semibold text-slate-500 uppercase">Max Tokens</label>
                    <input v-model.number="aiSettings.smart.max_tokens" type="number" min="1" placeholder="800" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                  </div>
                </div>
              </div>

              <div class="bg-white border border-slate-200/60 rounded-2xl p-5 shadow-sm space-y-4">
                <div class="flex items-center justify-between">
                  <h3 class="text-lg font-bold text-slate-900">Dumb AI</h3>
                  <span :class="['text-xs font-semibold px-2 py-1 rounded-full', aiEnabled.dumb ? 'bg-emerald-50 text-emerald-700' : 'bg-slate-100 text-slate-500']">
                    {{ aiEnabled.dumb ? 'Enabled' : 'Disabled' }}
                  </span>
                </div>

                <div class="flex items-center justify-between gap-3">
                  <p class="text-xs text-slate-500">Use for quick rewrites or simple edits.</p>
                  <button @click="copySmartToDumb" class="text-xs font-semibold text-brand-600 hover:text-brand-700">Copy smart → dumb</button>
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Provider</label>
                  <select v-model="aiSettings.dumb.provider" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                    <option value="">Select provider</option>
                    <option value="openai">OpenAI</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="gemini">Gemini</option>
                    <option value="ollama">Ollama</option>
                  </select>
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Model</label>
                  <input v-model="aiSettings.dumb.model" type="text" placeholder="gpt-4o-mini" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">API Key</label>
                  <input v-model="aiSettings.dumb.api_key" type="password" placeholder="sk-..." class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="space-y-2">
                  <label class="text-xs font-semibold text-slate-500 uppercase">Base URL</label>
                  <input v-model="aiSettings.dumb.base_url" type="text" placeholder="https://api.openai.com" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div class="space-y-2">
                    <label class="text-xs font-semibold text-slate-500 uppercase">Temperature</label>
                    <input v-model.number="aiSettings.dumb.temperature" type="number" step="0.1" min="0" max="2" placeholder="0.2" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                  </div>
                  <div class="space-y-2">
                    <label class="text-xs font-semibold text-slate-500 uppercase">Max Tokens</label>
                    <input v-model.number="aiSettings.dumb.max_tokens" type="number" min="1" placeholder="400" class="w-full text-sm p-2.5 border border-slate-200 rounded-lg focus:border-brand-500 focus:ring-1 focus:ring-brand-500 outline-none">
                  </div>
                </div>
              </div>
            </div>

            <div class="bg-white border border-slate-200/60 rounded-2xl p-5 shadow-sm text-sm text-slate-600">
              <p>Tip: If provider, model, or required API key is missing, the assistant is disabled. Ollama typically runs without an API key.</p>
            </div>
          </div>
        </div>

      </main>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { listPosts, createPost, updatePost, deletePost, getAISettings, updateAISettings, sendAIChat } from './api'
import MarkdownEditor from './components/MarkdownEditor.vue'

// --- State ---
const sidebarOpen = ref(false)
const currentView = ref('list') // 'list' or 'editor'
const searchQuery = ref('')
const filterStatus = ref('all') // 'all', 'published', 'draft'
const windowWidth = ref(window.innerWidth)
const loading = ref(false)
const saving = ref(false)
const posts = ref([])
const originalPostJson = ref('')
const aiSettings = ref(defaultAISettings())
const aiEnabled = ref({ smart: false, dumb: false })
const aiLoading = ref(false)
const aiSaving = ref(false)
const aiMode = ref('smart')
const aiQuery = ref('')
const aiBusy = ref(false)
const aiNotes = ref('')
const aiUseSearch = ref(false)
const aiHighlightEnabled = ref(true)
const aiHighlight = ref(null)

// Toast state
const toast = ref({
  show: false,
  message: '',
  type: 'success'
})

// Editor State
const draftPost = ref({
  id: null,
  title: '',
  slug: '',
  date: '',
  published: false,
  description: '',
  content: '',
  tags: []
})

// --- Computed ---
const isNewPost = computed(() => !draftPost.value.id)

const hasUnsavedChanges = computed(() => {
  return JSON.stringify(draftPost.value) !== originalPostJson.value
})

const filteredPosts = computed(() => {
  return posts.value.filter(post => {
    const matchesSearch = (post.title || '').toLowerCase().includes(searchQuery.value.toLowerCase()) || 
                          (post.slug || '').toLowerCase().includes(searchQuery.value.toLowerCase())
    
    if (filterStatus.value === 'all') return matchesSearch
    if (filterStatus.value === 'published') return matchesSearch && post.published_at
    if (filterStatus.value === 'draft') return matchesSearch && !post.published_at
    return matchesSearch
  })
})

const seoLengthColor = computed(() => {
  const len = draftPost.value.description.length
  if (len > 160) return 'text-rose-500 font-bold'
  if (len > 140) return 'text-amber-500'
  return 'text-slate-400'
})

// --- Methods ---
function showToast(message, type = 'success') {
  toast.value = { show: true, message, type }
  setTimeout(() => {
    toast.value.show = false
  }, 3000)
}

const formatDate = (dateStr) => {
  if (!dateStr) return 'Draft'
  return new Date(dateStr).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

async function loadPosts() {
  loading.value = true
  try {
    posts.value = await listPosts()
  } catch (err) {
    showToast('Failed to load posts: ' + err.message, 'error')
  } finally {
    loading.value = false
  }
}

async function loadAISettings() {
  aiLoading.value = true
  try {
    const result = await getAISettings()
    aiSettings.value = result?.settings ? normalizeAISettings(result.settings) : defaultAISettings()
    aiEnabled.value = {
      smart: !!result?.smart_enabled,
      dumb: !!result?.dumb_enabled
    }
    if (!aiEnabled.value.dumb && aiMode.value === 'dumb') {
      aiMode.value = 'smart'
    }
  } catch (err) {
    showToast('Failed to load AI settings: ' + err.message, 'error')
  } finally {
    aiLoading.value = false
  }
}

async function saveAISettings() {
  aiSaving.value = true
  try {
    const payload = normalizeAISettings(aiSettings.value)
    const result = await updateAISettings(payload)
    aiSettings.value = normalizeAISettings(result?.settings || payload)
    aiEnabled.value = {
      smart: !!result?.smart_enabled,
      dumb: !!result?.dumb_enabled
    }
    if (!aiEnabled.value.dumb && aiMode.value === 'dumb') {
      aiMode.value = 'smart'
    }
    showToast('AI settings saved')
  } catch (err) {
    showToast('Failed to save AI settings: ' + err.message, 'error')
  } finally {
    aiSaving.value = false
  }
}

function copySmartToDumb() {
  aiSettings.value.dumb = { ...aiSettings.value.smart }
}

async function sendAI() {
  if (!aiQuery.value.trim()) return
  if (aiMode.value === 'dumb' && !aiEnabled.value.dumb) {
    showToast('Dumb AI is not configured', 'error')
    return
  }
  if (!aiEnabled.value.smart && aiMode.value === 'smart') {
    showToast('Smart AI is not configured', 'error')
    return
  }

  aiBusy.value = true
  aiNotes.value = ''
  try {
    const result = await sendAIChat({
      mode: aiMode.value,
      content_markdown: draftPost.value.content,
      query: aiQuery.value,
      web_search: aiUseSearch.value
    })
    const nextContent = result?.content_markdown || ''
    if (!nextContent) {
      showToast('AI response was empty', 'error')
      return
    }
    const previous = draftPost.value.content
    draftPost.value.content = nextContent
    aiNotes.value = result?.notes || ''
    if (aiHighlightEnabled.value) {
      aiHighlight.value = { previous, current: nextContent, nonce: Date.now() }
    } else {
      aiHighlight.value = null
    }
    aiQuery.value = ''
    showToast('AI changes applied')
  } catch (err) {
    showToast('AI request failed: ' + err.message, 'error')
  } finally {
    aiBusy.value = false
  }
}

const createNewPost = () => {
  draftPost.value = {
    id: null,
    title: '',
    slug: '',
    date: new Date().toISOString().split('T')[0],
    published: false,
    description: '',
    content: '',
    tags: []
  }

  aiNotes.value = ''
  aiQuery.value = ''
  aiHighlight.value = null
  
  // Check for autosaved new post
  const autosaved = localStorage.getItem('autosave_new')
  if (autosaved) {
    try {
      const saved = JSON.parse(autosaved)
      if (confirm('Found an unsaved new post. Do you want to restore it?')) {
        draftPost.value = saved
      } else {
        localStorage.removeItem('autosave_new')
      }
    } catch (e) { console.error(e) }
  }

  originalPostJson.value = JSON.stringify(draftPost.value)
  currentView.value = 'editor'
}

const editPost = (post) => {
  // Map API post to editor format
  const mappedPost = {
    id: post.id,
    title: post.title || '',
    slug: post.slug || '',
    date: post.published_at ? post.published_at.split('T')[0] : new Date().toISOString().split('T')[0],
    published: !!post.published_at,
    description: post.meta_description || '',
    content: post.content_markdown || '',
    tags: post.tags || []
  }

  // Check for autosaved version
  const autosaveKey = `autosave_${post.id}`
  const autosaved = localStorage.getItem(autosaveKey)
  if (autosaved) {
    try {
      const saved = JSON.parse(autosaved)
      // Only restore if different from server version
      if (JSON.stringify(saved) !== JSON.stringify(mappedPost)) {
        if (confirm(`Found unsaved changes for this post from ${new Date().toLocaleDateString()}. Restore them?`)) {
            draftPost.value = saved
        } else {
            draftPost.value = mappedPost
            localStorage.removeItem(autosaveKey)
        }
      } else {
        draftPost.value = mappedPost
        localStorage.removeItem(autosaveKey)
      }
    } catch (e) { 
        draftPost.value = mappedPost
    }
  } else {
      draftPost.value = mappedPost
  }

  originalPostJson.value = JSON.stringify(draftPost.value)
  currentView.value = 'editor'
  aiQuery.value = ''
  aiNotes.value = ''
  aiHighlight.value = null
}

async function savePost() {
  if (!draftPost.value.title) {
    showToast('Title is required', 'error')
    return
  }
  if (!draftPost.value.slug) {
    showToast('Slug is required', 'error')
    return
  }

  saving.value = true
  
  try {
    // Convert editor format to API format
    const payload = {
      title: draftPost.value.title,
      slug: draftPost.value.slug,
      content_markdown: draftPost.value.content,
      content_html: DOMPurify.sanitize(marked.parse(draftPost.value.content || '')),
      meta_description: draftPost.value.description,
      published_at: draftPost.value.published ? new Date(draftPost.value.date).toISOString() : null,
      author_id: 1
    }

    if (draftPost.value.id) {
      await updatePost(draftPost.value.id, payload)
      showToast('Post updated successfully!')
      localStorage.removeItem(`autosave_${draftPost.value.id}`)
    } else {
      await createPost(payload)
      showToast('Post created successfully!')
      localStorage.removeItem('autosave_new')
    }
    
    await loadPosts()
    currentView.value = 'list'
  } catch (err) {
    showToast('Failed to save: ' + err.message, 'error')
  } finally {
    saving.value = false
  }
}

async function confirmDeletePost(post) {
  if (!confirm(`Delete post "${post.title || post.slug}"?`)) return
  
  try {
    await deletePost(post.id)
    await loadPosts()
    showToast('Post deleted successfully!')
  } catch (err) {
    showToast('Failed to delete: ' + err.message, 'error')
  }
}

const confirmBack = () => {
  if (hasUnsavedChanges.value) {
    if (!confirm("You have unsaved changes. Are you sure you want to leave?")) return
  }
  currentView.value = 'list'
}

const slugify = (text) => {
  return text.toString().toLowerCase()
    .replace(/\s+/g, '-')
    .replace(/[^\w\-]+/g, '')
    .replace(/\-\-+/g, '-')
    .replace(/^-+/, '')
    .replace(/-+$/, '')
}

const autoSlug = () => {
  if (isNewPost.value || !draftPost.value.slug) {
    draftPost.value.slug = slugify(draftPost.value.title)
  }
}

// --- Lifecycle & Watchers ---
const handleResize = () => {
  windowWidth.value = window.innerWidth
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  // Configure Marked
  marked.setOptions({
    breaks: true,
    gfm: true
  })
  // Load posts
  loadPosts()
  loadAISettings()
})

watch(draftPost, (newVal) => {
  if (currentView.value === 'editor') {
    const key = newVal.id ? `autosave_${newVal.id}` : 'autosave_new'
    // Only save if dirty
    if (JSON.stringify(newVal) !== originalPostJson.value) {
      localStorage.setItem(key, JSON.stringify(newVal))
    }
  }
}, { deep: true })

watch(aiHighlightEnabled, (enabled) => {
  if (!enabled) {
    aiHighlight.value = null
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})

function defaultAISettings() {
  return {
    smart: {
      provider: '',
      model: '',
      api_key: '',
      base_url: '',
      temperature: null,
      max_tokens: null
    },
    dumb: {
      provider: '',
      model: '',
      api_key: '',
      base_url: '',
      temperature: null,
      max_tokens: null
    }
  }
}

function normalizeAISettings(settings) {
  const smartTemp = Number.isFinite(settings?.smart?.temperature) ? settings.smart.temperature : null
  const smartMax = Number.isFinite(settings?.smart?.max_tokens) ? settings.smart.max_tokens : null
  const dumbTemp = Number.isFinite(settings?.dumb?.temperature) ? settings.dumb.temperature : null
  const dumbMax = Number.isFinite(settings?.dumb?.max_tokens) ? settings.dumb.max_tokens : null

  return {
    smart: {
      provider: settings?.smart?.provider || '',
      model: settings?.smart?.model || '',
      api_key: settings?.smart?.api_key || '',
      base_url: settings?.smart?.base_url || '',
      temperature: smartTemp,
      max_tokens: smartMax
    },
    dumb: {
      provider: settings?.dumb?.provider || '',
      model: settings?.dumb?.model || '',
      api_key: settings?.dumb?.api_key || '',
      base_url: settings?.dumb?.base_url || '',
      temperature: dumbTemp,
      max_tokens: dumbMax
    }
  }
}

</script>
